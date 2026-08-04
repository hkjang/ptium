package pptx

import (
	"math"
	"strings"
)

// maximumArtwork bounds how much of a layout's decoration is captured. Real
// templates settle well under this; a pathological one would otherwise bloat the
// stored manifest and slow every preview.
const maximumArtwork = 160

// artworkContext is what an extractor needs to resolve references: the theme and
// colour map for scheme colours, and the part's relationships for images.
type artworkContext struct {
	colorMap  map[string]string
	theme     Theme
	relations map[string]string
}

// collectArtwork walks a shape tree in paint order and records everything a
// preview can draw. Placeholders are skipped: they are the writable regions and
// are drawn from the deck's own content.
func collectArtwork(tree rawShapeTree, ctx artworkContext, into []Artwork) []Artwork {
	for _, child := range tree.Children {
		if len(into) >= maximumArtwork {
			return into
		}
		if child.Kind == "grpSp" && child.Group != nil {
			// A group remaps its children's coordinates onto its own frame.
			into = collectArtwork(projectGroup(*child.Group), ctx, into)
			continue
		}
		if child.Shape.placeholder() != nil {
			continue
		}
		if piece, ok := shapeArtwork(child.Shape, ctx); ok {
			into = append(into, piece)
		}
		// A shape can be both a filled box and a label; the text is a second piece
		// so it paints above its own background.
		if label, ok := textArtwork(child.Shape, ctx); ok && len(into) < maximumArtwork {
			into = append(into, label)
		}
	}
	return into
}

// projectGroup rewrites a group's children into slide coordinates. DrawingML
// gives a group a frame and a child coordinate space; without applying the
// mapping, every grouped shape lands in the wrong place and at the wrong size.
func projectGroup(group rawShapeTree) rawShapeTree {
	transform := group.Transform
	if transform == nil || transform.ChExt.CX <= 0 || transform.ChExt.CY <= 0 {
		return group
	}
	scaleX := float64(transform.Ext.CX) / float64(transform.ChExt.CX)
	scaleY := float64(transform.Ext.CY) / float64(transform.ChExt.CY)
	project := func(shape rawShape) rawShape {
		x, y, width, height, ok := shape.geometry()
		if !ok {
			return shape
		}
		mapped := &rawXfrm{}
		if source := shape.transform(); source != nil {
			*mapped = *source
		}
		mapped.Off.X = transform.Off.X + int(math.Round(float64(x-transform.ChOff.X)*scaleX))
		mapped.Off.Y = transform.Off.Y + int(math.Round(float64(y-transform.ChOff.Y)*scaleY))
		mapped.Ext.CX = int(math.Round(float64(width) * scaleX))
		mapped.Ext.CY = int(math.Round(float64(height) * scaleY))
		if shape.SpPr == nil {
			shape.SpPr = &rawShapeProp{}
		} else {
			properties := *shape.SpPr
			shape.SpPr = &properties
		}
		shape.SpPr.Xfrm = mapped
		shape.Xfrm = nil
		return shape
	}
	result := rawShapeTree{Transform: nil}
	for _, child := range group.Children {
		switch {
		case child.Kind == "grpSp" && child.Group != nil:
			nested := projectGroup(*child.Group)
			// Fold the parent's mapping into the already-projected children.
			for index := range nested.Children {
				if nested.Children[index].Group == nil {
					nested.Children[index].Shape = project(nested.Children[index].Shape)
				}
			}
			inner := nested
			result.Children = append(result.Children, rawTreeChild{Kind: "grpSp", Group: &inner})
		default:
			result.Children = append(result.Children, rawTreeChild{Kind: child.Kind, Shape: project(child.Shape)})
		}
	}
	return result
}

func shapeArtwork(shape rawShape, ctx artworkContext) (Artwork, bool) {
	x, y, width, height, ok := shape.geometry()
	if !ok || width <= 0 || height <= 0 {
		return Artwork{}, false
	}
	piece := Artwork{Kind: "shape", X: x, Y: y, Width: width, Height: height, Preset: shape.geometryPreset()}
	if transform := shape.transform(); transform != nil {
		piece.Rotation = transform.Rotation
		piece.FlipH = transform.FlipH == "1" || transform.FlipH == "true"
		piece.FlipV = transform.FlipV == "1" || transform.FlipV == "true"
	}
	properties := shape.SpPr
	if properties == nil {
		properties = &rawShapeProp{}
	}
	switch blip := shape.picture(); {
	case blip != nil:
		image, opacity := ctx.picture(blip)
		if image == "" {
			return Artwork{}, false
		}
		piece.Kind = "picture"
		piece.Image = image
		piece.Opacity = opacity
		if crop := blip.SrcRect; crop != nil {
			piece.Crop = [4]int{crop.L, crop.T, crop.R, crop.B}
		}
	case properties.GradFill != nil:
		piece.Gradient, piece.GradientAngle = ctx.gradient(properties.GradFill)
		if len(piece.Gradient) < 2 {
			return Artwork{}, false
		}
	case properties.SolidFill != nil:
		piece.Fill = resolveColorReference(shape.fill(), ctx.colorMap, ctx.theme)
		piece.Opacity = solidOpacity(properties.SolidFill)
		if piece.Fill == "" {
			return Artwork{}, false
		}
	}
	if line := properties.Line; line != nil && line.NoFill == nil && line.SolidFill != nil {
		piece.Stroke = resolveColorReference(solidFillColor(line.SolidFill), ctx.colorMap, ctx.theme)
		piece.StrokeWidth = line.Width
		if piece.StrokeWidth <= 0 {
			piece.StrokeWidth = 9525 // DrawingML's default hairline, 0.75pt.
		}
	}
	// A shape with no fill, no gradient, no image and no outline paints nothing.
	if piece.Fill == "" && piece.Image == "" && piece.Stroke == "" && len(piece.Gradient) == 0 {
		return Artwork{}, false
	}
	// Custom geometry is drawn as its bounding rectangle only when it is filled;
	// an unfilled custom outline is usually a detail a preview is better without.
	if properties.CustGeom != nil && piece.Fill == "" && piece.Image == "" && len(piece.Gradient) == 0 {
		return Artwork{}, false
	}
	return piece, true
}

func textArtwork(shape rawShape, ctx artworkContext) (Artwork, bool) {
	if shape.TxBody == nil || len(shape.TxBody.Para) == 0 {
		return Artwork{}, false
	}
	x, y, width, height, ok := shape.geometry()
	if !ok || width <= 0 || height <= 0 {
		return Artwork{}, false
	}
	var lines []string
	piece := Artwork{Kind: "text", X: x, Y: y, Width: width, Height: height,
		Anchor: shape.TxBody.BodyPr.Anchor}
	for _, paragraph := range shape.TxBody.Para {
		var line strings.Builder
		for _, run := range paragraph.Runs {
			line.WriteString(run.Text)
			if piece.FontSize == 0 && run.RPr.Size > 0 {
				piece.FontSize = run.RPr.Size
				piece.Bold = run.RPr.Bold == "1" || run.RPr.Bold == "true"
				piece.Font = run.RPr.Latin.Typeface
				if run.RPr.SolidFill != nil {
					piece.Color = resolveColorReference(solidFillColor(run.RPr.SolidFill), ctx.colorMap, ctx.theme)
				}
			}
		}
		if text := strings.TrimSpace(line.String()); text != "" {
			lines = append(lines, text)
			if piece.Align == "" {
				piece.Align = paragraph.PPr.Align
			}
		}
	}
	if len(lines) == 0 {
		return Artwork{}, false
	}
	// Slide-number and date fields are placeholders elsewhere; loose static text
	// is a brand line or a footer, and both are part of the design.
	piece.Text = strings.Join(lines, "\n")
	if piece.FontSize == 0 {
		piece.FontSize = 1200
	}
	if piece.Color == "" {
		piece.Color = resolveColorReference("tx1", ctx.colorMap, ctx.theme)
	}
	return piece, true
}

// picture resolves a blip fill to a package part and its opacity.
func (ctx artworkContext) picture(fill *rawBlipFill) (string, float64) {
	if fill == nil || fill.Blip.Embed == "" {
		return "", 0
	}
	part := ctx.relations[fill.Blip.Embed]
	if part == "" {
		return "", 0
	}
	opacity := 0.0
	if alpha := fill.Blip.Alpha; alpha != nil && alpha.Val > 0 && alpha.Val < 100000 {
		opacity = float64(alpha.Val) / 100000
	}
	return part, opacity
}

// gradient flattens a DrawingML gradient into ordered stops and an angle in
// degrees, resolving scheme colours and their luminance modifiers.
func (ctx artworkContext) gradient(fill *rawGradFill) ([]GradientStop, int) {
	if fill == nil {
		return nil, 0
	}
	stops := make([]GradientStop, 0, len(fill.GsLst.Gs))
	for _, stop := range fill.GsLst.Gs {
		entry := GradientStop{Position: float64(stop.Pos) / 100000}
		switch {
		case stop.SrgbClr != nil && stop.SrgbClr.Val != "":
			entry.Color = strings.ToUpper(stop.SrgbClr.Val)
			if alpha := stop.SrgbClr.Alpha; alpha != nil && alpha.Val > 0 && alpha.Val < 100000 {
				entry.Opacity = float64(alpha.Val) / 100000
			}
		case stop.SchemeClr != nil && stop.SchemeClr.Val != "":
			entry.Color = resolveColorReference(stop.SchemeClr.Val, ctx.colorMap, ctx.theme)
			if entry.Color == "" {
				continue
			}
			// Office states shades and tints relative to the scheme colour; a
			// gradient that ignores them collapses to two identical stops.
			if shade := stop.SchemeClr.Shade; shade != nil && shade.Val > 0 {
				entry.Color = mixColor(entry.Color, "000000", 1-float64(shade.Val)/100000)
			}
			if tint := stop.SchemeClr.Tint; tint != nil && tint.Val > 0 {
				entry.Color = mixColor(entry.Color, "FFFFFF", 1-float64(tint.Val)/100000)
			}
			if lum := stop.SchemeClr.LumMod; lum != nil && lum.Val > 0 && lum.Val != 100000 {
				entry.Color = mixColor(entry.Color, "000000", 1-float64(lum.Val)/100000)
			}
			if lum := stop.SchemeClr.LumOff; lum != nil && lum.Val > 0 {
				entry.Color = mixColor(entry.Color, "FFFFFF", float64(lum.Val)/100000)
			}
			if alpha := stop.SchemeClr.Alpha; alpha != nil && alpha.Val > 0 && alpha.Val < 100000 {
				entry.Opacity = float64(alpha.Val) / 100000
			}
		default:
			continue
		}
		stops = append(stops, entry)
	}
	if len(stops) < 2 {
		return nil, 0
	}
	angle := 90
	if fill.Lin != nil {
		angle = ((fill.Lin.Ang/60000)%360 + 360) % 360
	}
	return stops, angle
}

func solidFillColor(fill *rawSolidFill) string {
	if fill == nil {
		return ""
	}
	switch {
	case fill.SrgbClr != nil && fill.SrgbClr.Val != "":
		return strings.ToUpper(fill.SrgbClr.Val)
	case fill.SchemeClr != nil && fill.SchemeClr.Val != "":
		return fill.SchemeClr.Val
	case fill.SysClr != nil && fill.SysClr.LastClr != "":
		return strings.ToUpper(fill.SysClr.LastClr)
	}
	return ""
}

func solidOpacity(fill *rawSolidFill) float64 {
	if fill == nil {
		return 0
	}
	if fill.SrgbClr != nil && fill.SrgbClr.Alpha != nil {
		if value := fill.SrgbClr.Alpha.Val; value > 0 && value < 100000 {
			return float64(value) / 100000
		}
	}
	if fill.SchemeClr != nil && fill.SchemeClr.Alpha != nil {
		if value := fill.SchemeClr.Alpha.Val; value > 0 && value < 100000 {
			return float64(value) / 100000
		}
	}
	return 0
}

// background resolves a slide background, which may be a colour, a gradient or
// a picture.
func (ctx artworkContext) background(holder *rawFillHolder) (Background, bool) {
	if holder == nil {
		return Background{}, false
	}
	switch {
	case holder.BgPr.BlipFill != nil:
		if image, opacity := ctx.picture(holder.BgPr.BlipFill); image != "" {
			_ = opacity
			return Background{Image: image}, true
		}
	case holder.BgPr.GradFill != nil:
		if stops, angle := ctx.gradient(holder.BgPr.GradFill); len(stops) >= 2 {
			return Background{Gradient: stops, GradientAngle: angle}, true
		}
	}
	if color := resolveColorReference(holder.solidColor(), ctx.colorMap, ctx.theme); color != "" {
		return Background{Fill: color}, true
	}
	return Background{}, false
}
