package pdf

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

// DrawSVG paints a drawing onto a page.
//
// The drawing is the one the workspace already makes of every slide: the same
// code that draws the rail, the preview, the shared link and the presenting
// screen. Translating it is what keeps the printed page and the screen from
// drifting apart — a second renderer that laid slides out on its own would be
// two doors with one guard, and the PDF would quietly disagree with everything
// else about where a line sits.
//
// Only the shapes that drawing uses are understood. Anything else is skipped
// rather than guessed at.
func DrawSVG(page *Page, drawing string) error {
	decoder := xml.NewDecoder(strings.NewReader(drawing))
	painter := &svgPainter{page: page, gradients: map[string]gradient{}}
	// Gradients are defined before they are used, but a first pass costs
	// nothing and does not depend on that being true.
	painter.collectGradients(drawing)
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if err := painter.element(decoder, start); err != nil {
			return err
		}
	}
	return nil
}

type svgPainter struct {
	page      *Page
	gradients map[string]gradient
	// link is the address the text being drawn belongs to, if any.
	link string
}

type gradient struct{ from, to string }

func (p *svgPainter) collectGradients(drawing string) {
	decoder := xml.NewDecoder(strings.NewReader(drawing))
	id := ""
	var stops []string
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		switch element := token.(type) {
		case xml.StartElement:
			switch element.Name.Local {
			case "linearGradient":
				id, stops = attribute(element, "id"), nil
			case "stop":
				if id != "" {
					stops = append(stops, attribute(element, "stop-color"))
				}
			}
		case xml.EndElement:
			if element.Name.Local == "linearGradient" && id != "" && len(stops) > 0 {
				p.gradients[id] = gradient{from: stops[0], to: stops[len(stops)-1]}
				id = ""
			}
		}
	}
}

func (p *svgPainter) element(decoder *xml.Decoder, start xml.StartElement) error {
	switch start.Name.Local {
	case "rect":
		p.rect(start)
	case "ellipse", "circle":
		p.ellipse(start)
	case "polyline", "polygon":
		p.polyline(start, start.Name.Local == "polygon")
	case "path":
		p.path(start)
	case "line":
		p.line(start)
	case "image":
		p.image(start)
	case "a":
		p.link = attribute(start, "href")
		if p.link == "" {
			p.link = attribute(start, "{http://www.w3.org/1999/xlink}href")
		}
	case "text":
		return p.text(decoder, start)
	}
	return nil
}

func (p *svgPainter) rect(start xml.StartElement) {
	fill := p.fill(start)
	if fill == "" {
		return
	}
	p.page.Rect(number32(attribute(start, "x")), number32(attribute(start, "y")),
		number32(attribute(start, "width")), number32(attribute(start, "height")), fill)
}

func (p *svgPainter) ellipse(start xml.StartElement) {
	fill := p.fill(start)
	if fill == "" {
		return
	}
	cx, cy := number32(attribute(start, "cx")), number32(attribute(start, "cy"))
	rx, ry := number32(attribute(start, "rx")), number32(attribute(start, "ry"))
	if radius := number32(attribute(start, "r")); radius > 0 {
		rx, ry = radius, radius
	}
	p.page.Ellipse(cx, cy, rx, ry, fill)
}

func (p *svgPainter) polyline(start xml.StartElement, closed bool) {
	points := parsePoints(attribute(start, "points"))
	if len(points) < 2 {
		return
	}
	if fill := p.fill(start); closed && fill != "" {
		p.page.Polygon(points, fill)
		return
	}
	stroke := strings.TrimPrefix(attribute(start, "stroke"), "#")
	if stroke == "" || stroke == "none" {
		return
	}
	width := number32(attribute(start, "stroke-width"))
	if width <= 0 {
		width = 1
	}
	p.page.Polyline(points, stroke, width)
}

func (p *svgPainter) line(start xml.StartElement) {
	stroke := strings.TrimPrefix(attribute(start, "stroke"), "#")
	if stroke == "" || stroke == "none" {
		return
	}
	width := number32(attribute(start, "stroke-width"))
	if width <= 0 {
		width = 1
	}
	p.page.Polyline([]Position{
		{X: number32(attribute(start, "x1")), Y: number32(attribute(start, "y1"))},
		{X: number32(attribute(start, "x2")), Y: number32(attribute(start, "y2"))},
	}, stroke, width)
}

func (p *svgPainter) path(start xml.StartElement) {
	fill := p.fill(start)
	if fill == "" {
		return
	}
	if points := flattenPath(attribute(start, "d")); len(points) > 2 {
		p.page.Polygon(points, fill)
	}
}

func (p *svgPainter) image(start xml.StartElement) {
	source := attribute(start, "href")
	if source == "" {
		source = attribute(start, "{http://www.w3.org/1999/xlink}href")
	}
	image := decodeDataURI(source)
	if image == nil {
		return
	}
	p.page.Image(number32(attribute(start, "x")), number32(attribute(start, "y")),
		number32(attribute(start, "width")), number32(attribute(start, "height")), image)
}

// text walks one <text> element and the tspans inside it, which is where the
// drawing has already decided every line's place.
func (p *svgPainter) text(decoder *xml.Decoder, start xml.StartElement) error {
	size := number32(attribute(start, "font-size"))
	if size <= 0 {
		size = 12
	}
	color := strings.TrimPrefix(attribute(start, "fill"), "#")
	if color == "" {
		color = "000000"
	}
	bold := attribute(start, "font-weight") == "700" || attribute(start, "font-weight") == "bold"
	italic := attribute(start, "font-style") == "italic"
	anchor := attribute(start, "text-anchor")
	x, y := number32(attribute(start, "x")), number32(attribute(start, "y"))
	line := textRun{x: x, y: y, size: size, color: color, bold: bold, italic: italic, anchor: anchor}
	depth := 0
	for {
		token, err := decoder.Token()
		if err != nil {
			return nil
		}
		switch element := token.(type) {
		case xml.StartElement:
			depth++
			switch element.Name.Local {
			case "tspan":
				if value := attribute(element, "x"); value != "" {
					line.x = number32(value)
				}
				if value := attribute(element, "y"); value != "" {
					line.y = number32(value)
				}
				if value := attribute(element, "font-size"); value != "" {
					line.size = number32(value)
				}
				if value := attribute(element, "fill"); value != "" {
					line.color = strings.TrimPrefix(value, "#")
				}
				if value := attribute(element, "font-weight"); value != "" {
					line.bold = value == "700" || value == "bold"
				}
				if value := attribute(element, "font-style"); value != "" {
					line.italic = value == "italic"
				}
				line.underline = attribute(element, "text-decoration") == "underline"
			case "a":
				p.link = attribute(element, "href")
			}
		case xml.CharData:
			p.draw(line, string(element))
			line.x += p.page.document.TextWidth(string(element), line.size)
			line.underline = false
		case xml.EndElement:
			if element.Name.Local == "a" {
				p.link = ""
			}
			if element.Name.Local == "text" && depth == 0 {
				return nil
			}
			depth--
			if depth < 0 {
				return nil
			}
		}
	}
}

type textRun struct {
	x, y, size Point
	color      string
	bold       bool
	italic     bool
	underline  bool
	anchor     string
}

func (p *svgPainter) draw(line textRun, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	x := line.x
	// SVG hangs centred and right-set text off its own edge; PDF draws from
	// where the pen starts, so the width has to be taken off first.
	if width := p.page.document.TextWidth(text, line.size); line.anchor == "middle" {
		x -= width / 2
	} else if line.anchor == "end" {
		x -= width
	}
	width := p.page.Text(x, line.y, line.size, line.color, text, line.bold, line.italic)
	if line.underline {
		p.page.Underline(x, line.y, width, line.color, line.size)
	}
	if p.link != "" {
		target, page := p.link, 0
		if number, ok := strings.CutPrefix(target, "#slide-"); ok {
			if parsed, err := strconv.Atoi(number); err == nil {
				target, page = "", parsed
			}
		}
		p.page.Link(x, line.y-line.size*0.8, width, line.size*1.05, target, page)
	}
}

// fill is a shape's colour, resolving a gradient to the colour it starts from:
// a PDF can hold a gradient, and a slide's decoration reads the same either
// way, so the simpler file wins.
func (p *svgPainter) fill(start xml.StartElement) string {
	fill := attribute(start, "fill")
	if fill == "" || fill == "none" {
		return ""
	}
	if id, ok := strings.CutPrefix(fill, "url(#"); ok {
		if found, ok := p.gradients[strings.TrimSuffix(id, ")")]; ok {
			return strings.TrimPrefix(found.from, "#")
		}
		return ""
	}
	return strings.TrimPrefix(fill, "#")
}

func attribute(start xml.StartElement, name string) string {
	for _, attr := range start.Attr {
		if attr.Name.Local == name || attr.Name.Space+"|"+attr.Name.Local == name {
			return attr.Value
		}
		if name == "{http://www.w3.org/1999/xlink}href" && attr.Name.Local == "href" {
			return attr.Value
		}
	}
	return ""
}

func number32(value string) Point {
	value = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), "px"))
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func parsePoints(value string) []Position {
	var points []Position
	for _, pair := range strings.Fields(value) {
		parts := strings.Split(pair, ",")
		if len(parts) != 2 {
			continue
		}
		points = append(points, Position{X: number32(parts[0]), Y: number32(parts[1])})
	}
	return points
}

func decodeDataURI(source string) *Image {
	const marker = ";base64,"
	at := strings.Index(source, marker)
	if !strings.HasPrefix(source, "data:image/") || at < 0 {
		return nil
	}
	kind := source[len("data:"):at]
	raw, err := base64.StdEncoding.DecodeString(source[at+len(marker):])
	if err != nil || len(raw) == 0 {
		return nil
	}
	switch kind {
	case "image/jpeg", "image/jpg":
		return jpegImage(raw)
	case "image/png":
		return pngImage(raw)
	}
	return nil
}

var _ = fmt.Sprintf
