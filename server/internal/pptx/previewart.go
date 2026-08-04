package pptx

import (
	"fmt"
	"math"
	"strings"
)

// gradientRegistry collects the gradient definitions a preview needs. SVG wants
// them in a <defs> block, which is emitted once at the end.
type gradientRegistry struct {
	definitions []string
}

func (registry *gradientRegistry) add(stops []GradientStop, angle int) string {
	if len(stops) < 2 {
		return ""
	}
	id := fmt.Sprintf("g%d", len(registry.definitions)+1)
	// DrawingML measures the gradient direction clockwise from the positive x
	// axis; SVG wants the two end points.
	radians := float64(angle) * math.Pi / 180
	dx, dy := math.Cos(radians), math.Sin(radians)
	x1, y1 := 0.5-dx/2, 0.5-dy/2
	x2, y2 := 0.5+dx/2, 0.5+dy/2
	var builder strings.Builder
	fmt.Fprintf(&builder, `<linearGradient id="%s" x1="%.3f" y1="%.3f" x2="%.3f" y2="%.3f">`, id, x1, y1, x2, y2)
	for _, stop := range stops {
		opacity := ""
		if stop.Opacity > 0 && stop.Opacity < 1 {
			opacity = fmt.Sprintf(` stop-opacity="%.3f"`, stop.Opacity)
		}
		fmt.Fprintf(&builder, `<stop offset="%.3f" stop-color="#%s"%s/>`,
			math.Min(math.Max(stop.Position, 0), 1), escapeAttribute(stop.Color), opacity)
	}
	builder.WriteString(`</linearGradient>`)
	registry.definitions = append(registry.definitions, builder.String())
	return id
}

func (registry *gradientRegistry) defs() string {
	if len(registry.definitions) == 0 {
		return ""
	}
	return `<defs>` + strings.Join(registry.definitions, "") + `</defs>`
}

// previewBackground paints what the slide starts with: a colour, a gradient or a
// full-bleed picture.
func previewBackground(layout Layout, fallback string, pixelWidth, pixelHeight int, media MediaResolver, gradients *gradientRegistry) string {
	fill := layout.Fill
	switch {
	case fill.Image != "" && media != nil:
		if uri := media(fill.Image); uri != "" {
			return fmt.Sprintf(`<rect x="0" y="0" width="%d" height="%d" fill="#%s"/>`+
				`<image x="0" y="0" width="%d" height="%d" preserveAspectRatio="xMidYMid slice" href="%s"/>`,
				pixelWidth, pixelHeight, escapeAttribute(fallback), pixelWidth, pixelHeight, escapeAttribute(uri))
		}
	case len(fill.Gradient) >= 2:
		if id := gradients.add(fill.Gradient, fill.GradientAngle); id != "" {
			return fmt.Sprintf(`<rect x="0" y="0" width="%d" height="%d" fill="url(#%s)"/>`, pixelWidth, pixelHeight, id)
		}
	}
	color := fill.Fill
	if color == "" {
		color = fallback
	}
	return fmt.Sprintf(`<rect x="0" y="0" width="%d" height="%d" fill="#%s"/>`, pixelWidth, pixelHeight, escapeAttribute(color))
}

// previewArtwork draws one piece of a template's own decoration.
func previewArtwork(piece Artwork, scale float64, media MediaResolver, gradients *gradientRegistry) string {
	x := float64(piece.X) * scale
	y := float64(piece.Y) * scale
	width := float64(piece.Width) * scale
	height := float64(piece.Height) * scale
	if width <= 0.4 || height <= 0.4 {
		return ""
	}
	transform := artworkTransform(piece, x, y, width, height)

	switch piece.Kind {
	case "picture":
		if media == nil {
			return ""
		}
		uri := media(piece.Image)
		if uri == "" {
			return ""
		}
		opacity := ""
		if piece.Opacity > 0 && piece.Opacity < 1 {
			opacity = fmt.Sprintf(` opacity="%.3f"`, piece.Opacity)
		}
		// A cropped picture is drawn through a clip: the source rectangle says how
		// much of the image the frame shows.
		if piece.Crop != [4]int{} {
			return previewCroppedPicture(piece, x, y, width, height, uri, opacity, transform)
		}
		return fmt.Sprintf(`<image x="%.1f" y="%.1f" width="%.1f" height="%.1f" href="%s" preserveAspectRatio="none"%s%s/>`,
			x, y, width, height, escapeAttribute(uri), opacity, transform)
	case "text":
		return previewArtworkText(piece, x, y, width, height, scale, transform)
	}

	fill := "none"
	if id := gradients.add(piece.Gradient, piece.GradientAngle); id != "" {
		fill = "url(#" + id + ")"
	} else if piece.Fill != "" {
		fill = "#" + escapeAttribute(piece.Fill)
	}
	attributes := fmt.Sprintf(` fill="%s"`, fill)
	if piece.Opacity > 0 && piece.Opacity < 1 {
		attributes += fmt.Sprintf(` opacity="%.3f"`, piece.Opacity)
	}
	if piece.Stroke != "" {
		strokeWidth := math.Max(float64(piece.StrokeWidth)*scale, 0.6)
		attributes += fmt.Sprintf(` stroke="#%s" stroke-width="%.2f"`, escapeAttribute(piece.Stroke), strokeWidth)
	}
	return previewGeometry(piece.Preset, x, y, width, height, attributes+transform)
}

// previewGeometry renders the preset shapes a template actually decorates with.
// Anything unrecognised falls back to its bounding rectangle, which keeps the
// composition and colour of the design even when the exact outline is lost.
func previewGeometry(preset string, x, y, width, height float64, attributes string) string {
	switch preset {
	case "ellipse", "circle":
		return fmt.Sprintf(`<ellipse cx="%.1f" cy="%.1f" rx="%.1f" ry="%.1f"%s/>`,
			x+width/2, y+height/2, width/2, height/2, attributes)
	case "roundRect":
		return fmt.Sprintf(`<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="%.1f"%s/>`,
			x, y, width, height, math.Min(width, height)*0.16, attributes)
	case "round1Rect", "round2SameRect", "round2DiagRect", "snip1Rect", "snip2SameRect", "snip2DiagRect":
		return fmt.Sprintf(`<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="%.1f"%s/>`,
			x, y, width, height, math.Min(width, height)*0.1, attributes)
	case "triangle":
		return fmt.Sprintf(`<polygon points="%.1f,%.1f %.1f,%.1f %.1f,%.1f"%s/>`,
			x+width/2, y, x+width, y+height, x, y+height, attributes)
	case "rtTriangle":
		return fmt.Sprintf(`<polygon points="%.1f,%.1f %.1f,%.1f %.1f,%.1f"%s/>`,
			x, y, x, y+height, x+width, y+height, attributes)
	case "diamond":
		return fmt.Sprintf(`<polygon points="%.1f,%.1f %.1f,%.1f %.1f,%.1f %.1f,%.1f"%s/>`,
			x+width/2, y, x+width, y+height/2, x+width/2, y+height, x, y+height/2, attributes)
	case "line", "straightConnector1":
		// A line has no area; its frame is the segment it spans.
		return fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"%s/>`,
			x, y, x+width, y+height, strings.Replace(attributes, ` fill="none"`, "", 1))
	case "rightArrow":
		notch := height * 0.28
		return fmt.Sprintf(`<polygon points="%.1f,%.1f %.1f,%.1f %.1f,%.1f %.1f,%.1f %.1f,%.1f %.1f,%.1f %.1f,%.1f"%s/>`,
			x, y+notch, x+width*0.6, y+notch, x+width*0.6, y, x+width, y+height/2,
			x+width*0.6, y+height, x+width*0.6, y+height-notch, x, y+height-notch, attributes)
	case "chevron", "homePlate":
		inset := width * 0.22
		return fmt.Sprintf(`<polygon points="%.1f,%.1f %.1f,%.1f %.1f,%.1f %.1f,%.1f %.1f,%.1f"%s/>`,
			x, y, x+width-inset, y, x+width, y+height/2, x+width-inset, y+height, x, y+height, attributes)
	}
	return fmt.Sprintf(`<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f"%s/>`, x, y, width, height, attributes)
}

func previewCroppedPicture(piece Artwork, x, y, width, height float64, uri, opacity, transform string) string {
	left := float64(piece.Crop[0]) / 100000
	top := float64(piece.Crop[1]) / 100000
	right := float64(piece.Crop[2]) / 100000
	bottom := float64(piece.Crop[3]) / 100000
	visibleWidth := 1 - left - right
	visibleHeight := 1 - top - bottom
	if visibleWidth <= 0.01 || visibleHeight <= 0.01 {
		return ""
	}
	// Scale the image up so the visible window fills the frame, then clip to it.
	fullWidth := width / visibleWidth
	fullHeight := height / visibleHeight
	clipID := fmt.Sprintf("c%d%d", piece.X, piece.Y)
	return fmt.Sprintf(`<clipPath id="%s"><rect x="%.1f" y="%.1f" width="%.1f" height="%.1f"/></clipPath>`+
		`<g clip-path="url(#%s)"%s><image x="%.1f" y="%.1f" width="%.1f" height="%.1f" href="%s" preserveAspectRatio="none"%s/></g>`,
		clipID, x, y, width, height, clipID, transform,
		x-fullWidth*left, y-fullHeight*top, fullWidth, fullHeight, escapeAttribute(uri), opacity)
}

func previewArtworkText(piece Artwork, x, y, width, height, scale float64, transform string) string {
	lines := strings.Split(piece.Text, "\n")
	if len(lines) == 0 {
		return ""
	}
	// Hundredths of a point to points, then to the preview's pixels.
	fontSize := float64(piece.FontSize) / 100 * float64(EMUPerPoint) * scale
	if fontSize < 3 {
		return ""
	}
	lineHeight := fontSize * 1.2
	anchor, textX := "start", x
	switch piece.Align {
	case "ctr":
		anchor, textX = "middle", x+width/2
	case "r":
		anchor, textX = "end", x+width
	}
	textY := y + fontSize
	switch piece.Anchor {
	case "ctr":
		textY = y + height/2 - lineHeight*float64(len(lines)-1)/2 + fontSize*0.36
	case "b":
		textY = y + height - lineHeight*float64(len(lines)-1) - fontSize*0.2
	}
	weight := "400"
	if piece.Bold {
		weight = "700"
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, `<text x="%.1f" y="%.1f" fill="#%s" font-size="%.2f" font-weight="%s" text-anchor="%s" font-family="%s, Malgun Gothic, Apple SD Gothic Neo, sans-serif" xml:space="preserve"%s>`,
		textX, textY, escapeAttribute(piece.Color), fontSize, weight, anchor, escapeAttribute(fallbackFamily(piece.Font)), transform)
	for index, line := range lines {
		fmt.Fprintf(&builder, `<tspan x="%.1f" y="%.1f">%s</tspan>`, textX, textY+float64(index)*lineHeight, escapeText(line))
	}
	builder.WriteString(`</text>`)
	return builder.String()
}

// artworkTransform expresses rotation and flips as an SVG transform about the
// shape's own centre, which is where DrawingML applies them.
func artworkTransform(piece Artwork, x, y, width, height float64) string {
	var parts []string
	centerX, centerY := x+width/2, y+height/2
	if piece.Rotation != 0 {
		degrees := float64(piece.Rotation) / 60000
		parts = append(parts, fmt.Sprintf("rotate(%.2f %.1f %.1f)", degrees, centerX, centerY))
	}
	if piece.FlipH || piece.FlipV {
		scaleX, scaleY := 1.0, 1.0
		if piece.FlipH {
			scaleX = -1
		}
		if piece.FlipV {
			scaleY = -1
		}
		parts = append(parts, fmt.Sprintf("translate(%.1f %.1f) scale(%.0f %.0f) translate(%.1f %.1f)",
			centerX, centerY, scaleX, scaleY, -centerX, -centerY))
	}
	if len(parts) == 0 {
		return ""
	}
	return ` transform="` + strings.Join(parts, " ") + `"`
}
