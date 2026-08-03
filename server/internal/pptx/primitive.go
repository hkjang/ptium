package pptx

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// A slide component is laid out once into primitives and then emitted twice:
// as DrawingML for the exported file and as SVG for the browser preview. That
// is the only way the two can be guaranteed to agree.

// Frame is a rectangle in EMU.
type Frame struct {
	X, Y, Width, Height int
}

// Inset shrinks a frame on every side.
func (f Frame) Inset(amount int) Frame {
	return Frame{X: f.X + amount, Y: f.Y + amount, Width: f.Width - 2*amount, Height: f.Height - 2*amount}
}

// Right and Bottom are the far edges.
func (f Frame) Right() int  { return f.X + f.Width }
func (f Frame) Bottom() int { return f.Y + f.Height }

// Columns splits a frame into equal columns separated by a gap.
func (f Frame) Columns(count, gap int) []Frame {
	if count <= 0 {
		return nil
	}
	width := (f.Width - gap*(count-1)) / count
	result := make([]Frame, 0, count)
	for index := 0; index < count; index++ {
		result = append(result, Frame{X: f.X + index*(width+gap), Y: f.Y, Width: width, Height: f.Height})
	}
	return result
}

// Rows splits a frame into equal rows separated by a gap.
func (f Frame) Rows(count, gap int) []Frame {
	if count <= 0 {
		return nil
	}
	height := (f.Height - gap*(count-1)) / count
	result := make([]Frame, 0, count)
	for index := 0; index < count; index++ {
		result = append(result, Frame{X: f.X, Y: f.Y + index*(height+gap), Width: f.Width, Height: height})
	}
	return result
}

// Point is a coordinate in EMU.
type Point struct{ X, Y int }

// Primitive kinds.
const (
	shapeRectangle = "rect"
	shapeRounded   = "roundRect"
	shapeRound2    = "round2"
	shapeEllipse   = "ellipse"
	shapeChevron   = "chevron"
	shapePolyline  = "polyline"
	shapeText      = "text"
)

// Sides a two-corner rounded shape can round. A column rounds its top, a
// horizontal bar its right — the data end is rounded and the baseline stays
// square, so the bar visibly grows from the axis.
const (
	sideTop   = "top"
	sideRight = "right"
)

// Primitive is one drawn element: a filled shape, a stroked path or a text box.
type Primitive struct {
	Kind        string
	Frame       Frame
	Fill        string
	Stroke      string
	StrokeWidth int
	Corner      int
	Side        string
	Opacity     int // percent; zero means fully opaque
	Points      []Point

	// Text properties.
	Lines    []Paragraph
	FontSize int
	Color    string
	Bold     bool
	Align    string // l, ctr, r
	Anchor   string // t, ctr, b
	Font     string
	Wrap     bool
	Name     string
}

// Component is a laid-out slide element ready to be emitted.
type Component struct {
	Name       string
	Frame      Frame
	Primitives []Primitive
}

func filled(frame Frame, fill string) Primitive {
	return Primitive{Kind: shapeRectangle, Frame: frame, Fill: fill}
}

func rounded(frame Frame, fill string, corner int) Primitive {
	return Primitive{Kind: shapeRounded, Frame: frame, Fill: fill, Corner: corner}
}

func hairline(frame Frame, color string) Primitive {
	if frame.Height < 1 {
		frame.Height = 9525 // one device pixel at 96 dpi
	}
	return Primitive{Kind: shapeRectangle, Frame: frame, Fill: color}
}

func polyline(points []Point, color string, width int) Primitive {
	return Primitive{Kind: shapePolyline, Points: points, Stroke: color, StrokeWidth: width}
}

func dot(center Point, radius int, fill, ring string, ringWidth int) []Primitive {
	result := make([]Primitive, 0, 2)
	if ring != "" && ringWidth > 0 {
		outer := radius + ringWidth
		result = append(result, Primitive{Kind: shapeEllipse,
			Frame: Frame{X: center.X - outer, Y: center.Y - outer, Width: 2 * outer, Height: 2 * outer}, Fill: ring})
	}
	result = append(result, Primitive{Kind: shapeEllipse,
		Frame: Frame{X: center.X - radius, Y: center.Y - radius, Width: 2 * radius, Height: 2 * radius}, Fill: fill})
	return result
}

type textOptions struct {
	Size   int
	Color  string
	Bold   bool
	Align  string
	Anchor string
	Font   string
	Wrap   bool
}

func text(frame Frame, lines []Paragraph, options textOptions) Primitive {
	if options.Align == "" {
		options.Align = "l"
	}
	if options.Anchor == "" {
		options.Anchor = "t"
	}
	return Primitive{Kind: shapeText, Frame: frame, Lines: lines, FontSize: options.Size,
		Color: options.Color, Bold: options.Bold, Align: options.Align, Anchor: options.Anchor,
		Font: options.Font, Wrap: options.Wrap}
}

func line(value string) []Paragraph { return []Paragraph{{Text: value}} }

// --- DrawingML emitter ------------------------------------------------------

// DrawingML wraps a component in a group whose child coordinate space matches
// its own, so children keep absolute coordinates while the whole component
// stays selectable and movable as one object in PowerPoint.
func (c Component) DrawingML(startID int) (string, int) {
	if len(c.Primitives) == 0 {
		return "", startID
	}
	id := startID
	var body strings.Builder
	for _, primitive := range c.Primitives {
		id++
		body.WriteString(primitive.drawingML(id))
	}
	groupID := startID
	name := c.Name
	if name == "" {
		name = "Component"
	}
	group := fmt.Sprintf(`<p:grpSp><p:nvGrpSpPr><p:cNvPr id="%d" name="%s"/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>`+
		`<p:grpSpPr><a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/><a:chOff x="%d" y="%d"/><a:chExt cx="%d" cy="%d"/></a:xfrm></p:grpSpPr>%s</p:grpSp>`,
		groupID, escapeAttribute(name),
		c.Frame.X, c.Frame.Y, c.Frame.Width, c.Frame.Height,
		c.Frame.X, c.Frame.Y, c.Frame.Width, c.Frame.Height, body.String())
	return group, id + 1
}

// bar builds a data mark whose value end is rounded and whose baseline is
// square.
func bar(frame Frame, fill string, corner int, side string) Primitive {
	return Primitive{Kind: shapeRound2, Frame: frame, Fill: fill, Corner: corner, Side: side}
}

func (p Primitive) drawingML(id int) string {
	switch p.Kind {
	case shapeText:
		return p.textDrawingML(id)
	case shapePolyline:
		return p.polylineDrawingML(id)
	}
	geometry := `<a:prstGeom prst="rect"><a:avLst/></a:prstGeom>`
	switch p.Kind {
	case shapeRounded:
		adjust := 16667
		if p.Frame.Height > 0 && p.Corner > 0 {
			shorter := p.Frame.Height
			if p.Frame.Width < shorter {
				shorter = p.Frame.Width
			}
			adjust = int(float64(p.Corner) / float64(shorter) * 100000)
			if adjust > 50000 {
				adjust = 50000
			}
			if adjust < 1000 {
				adjust = 1000
			}
		}
		geometry = `<a:prstGeom prst="roundRect"><a:avLst><a:gd name="adj" fmla="val ` + strconv.Itoa(adjust) + `"/></a:avLst></a:prstGeom>`
	case shapeEllipse:
		geometry = `<a:prstGeom prst="ellipse"><a:avLst/></a:prstGeom>`
	case shapeChevron:
		geometry = `<a:prstGeom prst="chevron"><a:avLst/></a:prstGeom>`
	case shapeRound2:
		geometry = `<a:prstGeom prst="round2SameRect"><a:avLst><a:gd name="adj1" fmla="val ` +
			strconv.Itoa(cornerAdjustment(p)) + `"/><a:gd name="adj2" fmla="val 0"/></a:avLst></a:prstGeom>`
	}
	fill := `<a:noFill/>`
	if p.Fill != "" {
		fill = `<a:solidFill>` + solidColor(p.Fill, p.Opacity) + `</a:solidFill>`
	}
	outline := `<a:ln><a:noFill/></a:ln>`
	if p.Stroke != "" {
		width := p.StrokeWidth
		if width <= 0 {
			width = 9525
		}
		outline = `<a:ln w="` + strconv.Itoa(width) + `" cap="rnd"><a:solidFill>` + solidColor(p.Stroke, 0) + `</a:solidFill></a:ln>`
	}
	name := p.Name
	if name == "" {
		name = "Shape"
	}
	return fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="%d" name="%s %d"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>`+
		`<p:spPr>%s%s%s%s</p:spPr>`+
		`<p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:endParaRPr lang="ko-KR"/></a:p></p:txBody></p:sp>`,
		id, escapeAttribute(name), id, p.transform(), geometry, fill, outline)
}

// transform emits the shape's placement. A right-rounded bar is authored as a
// top-rounded column turned a quarter turn about its own centre, because
// PowerPoint's two-corner rounded rectangle always rounds the top pair.
func (p Primitive) transform() string {
	frame, rotation := p.Frame, 0
	if p.Kind == shapeRound2 && p.Side == sideRight {
		centerX, centerY := frame.X+frame.Width/2, frame.Y+frame.Height/2
		frame = Frame{X: centerX - frame.Height/2, Y: centerY - frame.Width/2, Width: frame.Height, Height: frame.Width}
		rotation = 5400000
	}
	attribute := ""
	if rotation != 0 {
		attribute = fmt.Sprintf(` rot="%d"`, rotation)
	}
	return fmt.Sprintf(`<a:xfrm%s><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm>`,
		attribute, frame.X, frame.Y, frame.Width, frame.Height)
}

// cornerAdjustment converts a corner radius in EMU into the fraction of the
// shorter side that PowerPoint's adjustment value expects.
func cornerAdjustment(p Primitive) int {
	shorter := p.Frame.Height
	if p.Kind == shapeRound2 && p.Side == sideRight {
		shorter = p.Frame.Width
	}
	if p.Frame.Width < shorter {
		shorter = p.Frame.Width
	}
	if p.Kind == shapeRound2 && p.Side == sideRight && p.Frame.Height < shorter {
		shorter = p.Frame.Height
	}
	if shorter <= 0 || p.Corner <= 0 {
		return 8000
	}
	adjust := int(float64(p.Corner) / float64(shorter) * 100000)
	if adjust > 50000 {
		return 50000
	}
	if adjust < 500 {
		return 500
	}
	return adjust
}

func (p Primitive) textDrawingML(id int) string {
	wrap := "none"
	if p.Wrap {
		wrap = "square"
	}
	name := p.Name
	if name == "" {
		name = "Text"
	}
	var paragraphs strings.Builder
	for _, paragraph := range p.Lines {
		properties := `<a:pPr algn="` + p.Align + `"`
		if paragraph.Level > 0 {
			properties += ` lvl="` + strconv.Itoa(paragraph.Level) + `"`
		}
		properties += `><a:buNone/></a:pPr>`
		runProperties := `<a:rPr lang="ko-KR" sz="` + strconv.Itoa(p.FontSize) + `" dirty="0"`
		if p.Bold {
			runProperties += ` b="1"`
		}
		runProperties += `><a:solidFill>` + solidColor(p.Color, 0) + `</a:solidFill>`
		if strings.TrimSpace(p.Font) != "" && !strings.HasPrefix(p.Font, "+") {
			runProperties += `<a:latin typeface="` + escapeAttribute(p.Font) + `"/><a:ea typeface="` + escapeAttribute(p.Font) + `"/>`
		}
		runProperties += `</a:rPr>`
		paragraphs.WriteString(`<a:p>` + properties + `<a:r>` + runProperties + `<a:t>` + escapeText(paragraph.Text) + `</a:t></a:r></a:p>`)
	}
	if paragraphs.Len() == 0 {
		paragraphs.WriteString(`<a:p><a:endParaRPr lang="ko-KR"/></a:p>`)
	}
	return fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="%d" name="%s %d"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr>`+
		`<p:spPr><a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:noFill/></p:spPr>`+
		`<p:txBody><a:bodyPr wrap="%s" lIns="0" tIns="0" rIns="0" bIns="0" anchor="%s"><a:spAutoFit/></a:bodyPr><a:lstStyle/>%s</p:txBody></p:sp>`,
		id, escapeAttribute(name), id, p.Frame.X, p.Frame.Y, p.Frame.Width, p.Frame.Height, wrap, p.Anchor, paragraphs.String())
}

// polylineDrawingML emits a stroked freeform path, which is how a line series
// is drawn without embedding a chart part and its workbook.
func (p Primitive) polylineDrawingML(id int) string {
	if len(p.Points) < 2 {
		return ""
	}
	minimumX, minimumY := p.Points[0].X, p.Points[0].Y
	maximumX, maximumY := minimumX, minimumY
	for _, point := range p.Points[1:] {
		minimumX = min(minimumX, point.X)
		minimumY = min(minimumY, point.Y)
		maximumX = max(maximumX, point.X)
		maximumY = max(maximumY, point.Y)
	}
	width, height := max(maximumX-minimumX, 1), max(maximumY-minimumY, 1)
	var path strings.Builder
	for index, point := range p.Points {
		verb := "lnTo"
		if index == 0 {
			verb = "moveTo"
		}
		fmt.Fprintf(&path, `<a:%s><a:pt x="%d" y="%d"/></a:%s>`, verb, point.X-minimumX, point.Y-minimumY, verb)
	}
	strokeWidth := p.StrokeWidth
	if strokeWidth <= 0 {
		strokeWidth = 2 * 9525
	}
	return fmt.Sprintf(`<p:sp><p:nvSpPr><p:cNvPr id="%d" name="Series %d"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>`+
		`<p:spPr><a:xfrm><a:off x="%d" y="%d"/><a:ext cx="%d" cy="%d"/></a:xfrm>`+
		`<a:custGeom><a:avLst/><a:gdLst/><a:ahLst/><a:cxnLst/><a:rect l="0" t="0" r="r" b="b"/>`+
		`<a:pathLst><a:path w="%d" h="%d">%s</a:path></a:pathLst></a:custGeom><a:noFill/>`+
		`<a:ln w="%d" cap="rnd"><a:solidFill>%s</a:solidFill><a:round/></a:ln></p:spPr>`+
		`<p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:endParaRPr lang="ko-KR"/></a:p></p:txBody></p:sp>`,
		id, id, minimumX, minimumY, width, height, width, height, path.String(), strokeWidth, solidColor(p.Stroke, 0))
}

func solidColor(value string, opacityPercent int) string {
	value = strings.ToUpper(strings.TrimPrefix(strings.TrimSpace(value), "#"))
	if !hexColorPattern.MatchString(value) {
		value = "808080"
	}
	if opacityPercent > 0 && opacityPercent < 100 {
		return `<a:srgbClr val="` + value + `"><a:alpha val="` + strconv.Itoa(opacityPercent*1000) + `"/></a:srgbClr>`
	}
	return `<a:srgbClr val="` + value + `"/>`
}

// --- SVG emitter ------------------------------------------------------------

// SVG emits the same component for the browser preview. Coordinates are scaled
// from EMU to CSS pixels by the caller's scale factor.
func (c Component) SVG(scale float64) string {
	var builder strings.Builder
	for _, primitive := range c.Primitives {
		builder.WriteString(primitive.svg(scale))
	}
	return builder.String()
}

func (p Primitive) svg(scale float64) string {
	position := func(value int) float64 { return float64(value) * scale }
	switch p.Kind {
	case shapeText:
		return p.textSVG(scale)
	case shapePolyline:
		if len(p.Points) < 2 {
			return ""
		}
		coordinates := make([]string, 0, len(p.Points))
		for _, point := range p.Points {
			coordinates = append(coordinates, fmt.Sprintf("%.1f,%.1f", position(point.X), position(point.Y)))
		}
		width := float64(p.StrokeWidth) * scale
		if width < 1 {
			width = 1.6
		}
		return fmt.Sprintf(`<polyline points="%s" fill="none" stroke="#%s" stroke-width="%.1f" stroke-linejoin="round" stroke-linecap="round"/>`,
			strings.Join(coordinates, " "), colorOrGrey(p.Stroke), width)
	case shapeEllipse:
		return fmt.Sprintf(`<ellipse cx="%.1f" cy="%.1f" rx="%.1f" ry="%.1f" fill="#%s"%s/>`,
			position(p.Frame.X+p.Frame.Width/2), position(p.Frame.Y+p.Frame.Height/2),
			position(p.Frame.Width)/2, position(p.Frame.Height)/2, colorOrGrey(p.Fill), svgOpacity(p.Opacity))
	case shapeRound2:
		return p.round2SVG(scale)
	}
	radius := 0.0
	if p.Kind == shapeRounded {
		radius = position(p.Corner)
	}
	fill := `fill="none"`
	if p.Fill != "" {
		fill = fmt.Sprintf(`fill="#%s"`, colorOrGrey(p.Fill))
	}
	stroke := ""
	if p.Stroke != "" {
		width := float64(p.StrokeWidth) * scale
		if width < 1 {
			width = 1
		}
		stroke = fmt.Sprintf(` stroke="#%s" stroke-width="%.1f"`, colorOrGrey(p.Stroke), width)
	}
	return fmt.Sprintf(`<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="%.1f" %s%s%s/>`,
		position(p.Frame.X), position(p.Frame.Y), position(p.Frame.Width), position(p.Frame.Height),
		radius, fill, stroke, svgOpacity(p.Opacity))
}

func (p Primitive) textSVG(scale float64) string {
	fontSize := float64(p.FontSize) / 100 * float64(EMUPerPoint) * scale
	if fontSize < 4 {
		fontSize = 4
	}
	lineHeight := fontSize * 1.24
	anchor := map[string]string{"l": "start", "ctr": "middle", "r": "end"}[p.Align]
	if anchor == "" {
		anchor = "start"
	}
	x := float64(p.Frame.X) * scale
	switch p.Align {
	case "ctr":
		x += float64(p.Frame.Width) * scale / 2
	case "r":
		x += float64(p.Frame.Width) * scale
	}
	// Wrap first so vertical anchoring accounts for the real line count.
	lineEm := float64(p.Frame.Width) / (float64(p.FontSize) / 100 * float64(EMUPerPoint))
	if !p.Wrap || lineEm < 1 {
		lineEm = 1e6
	}
	wrapped := make([]string, 0, len(p.Lines))
	for _, paragraph := range p.Lines {
		pieces := wrapText(paragraph.Text, lineEm)
		if len(pieces) == 0 {
			pieces = []string{""}
		}
		wrapped = append(wrapped, pieces...)
	}
	block := float64(len(wrapped)) * lineHeight
	top := float64(p.Frame.Y) * scale
	switch p.Anchor {
	case "ctr":
		top += (float64(p.Frame.Height)*scale - block) / 2
	case "b":
		top += float64(p.Frame.Height)*scale - block
	}
	weight := "400"
	if p.Bold {
		weight = "700"
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, `<text x="%.1f" y="%.1f" fill="#%s" font-size="%.2f" font-weight="%s" text-anchor="%s" font-family="%s, Malgun Gothic, Apple SD Gothic Neo, sans-serif" xml:space="preserve">`,
		x, top+fontSize*0.82, colorOrGrey(p.Color), fontSize, weight, anchor, escapeAttribute(fallbackFamily(p.Font)))
	for index, value := range wrapped {
		fmt.Fprintf(&builder, `<tspan x="%.1f" y="%.1f">%s</tspan>`, x, top+fontSize*0.82+float64(index)*lineHeight, escapeText(value))
	}
	builder.WriteString(`</text>`)
	return builder.String()
}

// round2SVG draws a bar with only its data end rounded. SVG's rect rounds all
// four corners, so the mark is a path.
func (p Primitive) round2SVG(scale float64) string {
	x := float64(p.Frame.X) * scale
	y := float64(p.Frame.Y) * scale
	width := float64(p.Frame.Width) * scale
	height := float64(p.Frame.Height) * scale
	radius := float64(p.Corner) * scale
	limit := math.Min(width, height) / 2
	if radius > limit {
		radius = limit
	}
	if radius < 0.5 {
		return fmt.Sprintf(`<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="#%s"%s/>`,
			x, y, width, height, colorOrGrey(p.Fill), svgOpacity(p.Opacity))
	}
	var path string
	if p.Side == sideRight {
		path = fmt.Sprintf("M%.1f %.1f H%.1f a%.1f %.1f 0 0 1 %.1f %.1f V%.1f a%.1f %.1f 0 0 1 %.1f %.1f H%.1f Z",
			x, y, x+width-radius, radius, radius, radius, radius,
			y+height-radius, radius, radius, -radius, radius, x)
	} else {
		path = fmt.Sprintf("M%.1f %.1f V%.1f a%.1f %.1f 0 0 1 %.1f %.1f H%.1f a%.1f %.1f 0 0 1 %.1f %.1f V%.1f Z",
			x, y+height, y+radius, radius, radius, radius, -radius,
			x+width-radius, radius, radius, radius, radius, y+height)
	}
	return fmt.Sprintf(`<path d="%s" fill="#%s"%s/>`, path, colorOrGrey(p.Fill), svgOpacity(p.Opacity))
}

func svgOpacity(percent int) string {
	if percent <= 0 || percent >= 100 {
		return ""
	}
	return fmt.Sprintf(` opacity="%.2f"`, float64(percent)/100)
}

func colorOrGrey(value string) string {
	value = strings.ToUpper(strings.TrimPrefix(strings.TrimSpace(value), "#"))
	if !hexColorPattern.MatchString(value) {
		return "808080"
	}
	return value
}
