package pdf

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Point is a PDF unit: 1/72 inch. A slide is measured in EMU, and the caller
// converts once rather than every drawing doing it again.
type Point = float64

// Document is a deck on paper. Pages are all the same size, because a deck is.
type Document struct {
	Width, Height Point
	Title         string
	font          *TrueType
	fontName      string
	used          map[uint16]rune
	undrawn       map[rune]int
	pages         []*Page
	images        []*imageXObject
	imageNames    map[string]string
	shadings      []shading
}

// New starts a document of the given page size, drawn in the given font.
func New(width, height Point, title string, font *TrueType) *Document {
	return &Document{Width: width, Height: height, Title: title, font: font,
		fontName: "F1", used: map[uint16]rune{}, undrawn: map[rune]int{}}
}

// Page is one slide's worth of drawing. Coordinates come in the way a slide is
// measured — x from the left, y from the top — and are turned over here, so
// nothing above this has to think in PDF's upside-down page.
type Page struct {
	document *Document
	content  bytes.Buffer
	links    []link
	// What this page draws with, so its resources name what it uses and not
	// what every other page happens to use.
	images   []string
	shadings []string
}

type link struct {
	x, y, width, height Point
	target              string
	page                int
}

type imageXObject struct {
	name       string
	width      int
	height     int
	data       []byte
	colorSpace string
	filter     string
	bits       int
	softMask   []byte
}

// AddPage starts a new page and returns it.
func (d *Document) AddPage() *Page {
	page := &Page{document: d}
	d.pages = append(d.pages, page)
	return page
}

func (p *Page) flip(y Point) Point { return p.document.Height - y }

// Rect fills a rectangle. Colours are "RRGGBB" as everything else in the
// workspace states them.
func (p *Page) Rect(x, y, width, height Point, color string) {
	if width <= 0 || height <= 0 {
		return
	}
	red, green, blue := rgb(color)
	fmt.Fprintf(&p.content, "%s %s %s rg %s %s %s %s re f\n",
		number(red), number(green), number(blue),
		number(x), number(p.flip(y+height)), number(width), number(height))
}

// Text draws one run at a baseline. The size is in points; bold is drawn by
// stroking the glyphs, which is what a renderer does when it has one weight of
// a face and needs two.
func (p *Page) Text(x, y Point, size Point, color, text string, bold, italic bool) Point {
	if strings.TrimSpace(text) == "" {
		return 0
	}
	font := p.document.font
	var glyphs strings.Builder
	width := 0.0
	for _, character := range text {
		glyph, ok := font.Glyph(character)
		if !ok {
			// A character this font cannot draw is drawn as a space rather than
			// as glyph 0, which most readers draw as a hollow box. It is counted
			// on the way past: a word that leaves the page as blanks is a thing
			// the author has to be told, not something to find in print.
			p.document.undrawn[character]++
			glyph, ok = font.Glyph(' ')
			if !ok {
				continue
			}
		}
		p.document.used[glyph] = character
		fmt.Fprintf(&glyphs, "%04X", glyph)
		width += float64(font.Width(glyph)) / 1000 * size
	}
	if glyphs.Len() == 0 {
		return 0
	}
	red, green, blue := rgb(color)
	p.content.WriteString("BT\n")
	if italic {
		// A slant applied to the text matrix: the face has no italic of its own.
		fmt.Fprintf(&p.content, "1 0 0.21256 1 %s %s Tm\n", number(x), number(p.flip(y)))
	} else {
		fmt.Fprintf(&p.content, "1 0 0 1 %s %s Tm\n", number(x), number(p.flip(y)))
	}
	fmt.Fprintf(&p.content, "/%s %s Tf\n", p.document.fontName, number(size))
	fmt.Fprintf(&p.content, "%s %s %s rg\n", number(red), number(green), number(blue))
	if bold {
		fmt.Fprintf(&p.content, "%s %s %s RG %s w 2 Tr\n",
			number(red), number(green), number(blue), number(size*0.032))
	}
	fmt.Fprintf(&p.content, "<%s> Tj\nET\n", glyphs.String())
	return width
}

// Undrawn is every character the font had no glyph for, with how many times it
// was met. A PDF built from a deck written in a script the built-in face does
// not cover comes out with those characters missing, and nothing on the page
// says so.
func (d *Document) Undrawn() map[rune]int {
	if len(d.undrawn) == 0 {
		return nil
	}
	out := make(map[rune]int, len(d.undrawn))
	for character, count := range d.undrawn {
		out[character] = count
	}
	return out
}

// TextWidth is how wide a run would be drawn, without drawing it.
func (d *Document) TextWidth(text string, size Point) Point {
	width := 0.0
	for _, character := range text {
		glyph, ok := d.font.Glyph(character)
		if !ok {
			glyph, _ = d.font.Glyph(' ')
		}
		width += float64(d.font.Width(glyph)) / 1000 * size
	}
	return width
}

// Underline draws the line under a link, since a PDF link is an area of the
// page rather than something drawn.
func (p *Page) Underline(x, y, width Point, color string, size Point) {
	red, green, blue := rgb(color)
	fmt.Fprintf(&p.content, "%s %s %s RG %s w %s %s m %s %s l S\n",
		number(red), number(green), number(blue), number(size*0.05),
		number(x), number(p.flip(y+size*0.12)), number(x+width), number(p.flip(y+size*0.12)))
}

// Link makes an area of the page click through, either out of the document or
// to one of its own pages (1-based, 0 for none).
func (p *Page) Link(x, y, width, height Point, target string, page int) {
	p.links = append(p.links, link{x: x, y: y, width: width, height: height, target: target, page: page})
}

// Image draws an already-encoded image. JPEG bytes are carried through as they
// are; anything else is given as raw samples by the caller.
func (p *Page) Image(x, y, width, height Point, image *Image) {
	if image == nil || width <= 0 || height <= 0 {
		return
	}
	// The same picture on twenty slides is one picture. A deck's logo, a divider's
	// background, a photograph carried through a section: the drawing of each
	// slide brings its own copy of the bytes, and embedding each one would make a
	// forty-slide deck forty times heavier than it is.
	key := imageKey(image)
	name, known := p.document.imageNames[key]
	if !known {
		name = fmt.Sprintf("Im%d", len(p.document.images)+1)
		p.document.images = append(p.document.images, &imageXObject{name: name,
			width: image.Width, height: image.Height, data: image.Data,
			colorSpace: image.ColorSpace, filter: image.Filter, bits: image.Bits,
			softMask: image.Alpha})
		if p.document.imageNames == nil {
			p.document.imageNames = map[string]string{}
		}
		p.document.imageNames[key] = name
	}
	if !slices.Contains(p.images, name) {
		p.images = append(p.images, name)
	}
	fmt.Fprintf(&p.content, "q %s 0 0 %s %s %s cm /%s Do Q\n",
		number(width), number(height), number(x), number(p.flip(y+height)), name)
}

// Image is a picture ready to be put on a page.
type Image struct {
	Width, Height int
	Data          []byte
	ColorSpace    string
	Filter        string
	Bits          int
	// Alpha is an 8-bit mask, or nil for a picture with none.
	Alpha []byte
}

// imageKey is what makes two pictures the same picture: the bytes, and what
// they are to be read as.
func imageKey(image *Image) string {
	sum := sha256.Sum256(image.Data)
	mask := ""
	if len(image.Alpha) > 0 {
		alpha := sha256.Sum256(image.Alpha)
		mask = hex.EncodeToString(alpha[:8])
	}
	return fmt.Sprintf("%s-%dx%d-%s-%s-%d-%s", hex.EncodeToString(sum[:16]),
		image.Width, image.Height, image.ColorSpace, image.Filter, image.Bits, mask)
}

func rgb(color string) (float64, float64, float64) {
	color = strings.TrimPrefix(strings.TrimSpace(color), "#")
	if len(color) != 6 {
		return 0, 0, 0
	}
	value, err := strconv.ParseUint(color, 16, 32)
	if err != nil {
		return 0, 0, 0
	}
	return float64((value>>16)&0xFF) / 255, float64((value>>8)&0xFF) / 255, float64(value&0xFF) / 255
}

// number writes a coordinate the way a PDF wants it: no exponent, no more
// precision than a printer can use.
func number(value float64) string {
	return strconv.FormatFloat(value, 'f', 3, 64)
}

func deflate(data []byte) []byte {
	var out bytes.Buffer
	writer := zlib.NewWriter(&out)
	_, _ = writer.Write(data)
	_ = writer.Close()
	return out.Bytes()
}

func sortedGlyphs(used map[uint16]rune) []uint16 {
	glyphs := make([]uint16, 0, len(used))
	for glyph := range used {
		glyphs = append(glyphs, glyph)
	}
	sort.Slice(glyphs, func(a, b int) bool { return glyphs[a] < glyphs[b] })
	return glyphs
}

// Position is a point on the page, measured the way a slide is: from the top
// left.
type Position struct{ X, Y Point }

// Ellipse fills an ellipse, drawn as the four bezier arcs every renderer uses
// for one — a PDF has no ellipse of its own.
func (p *Page) Ellipse(cx, cy, rx, ry Point, color string) {
	if rx <= 0 || ry <= 0 {
		return
	}
	red, green, blue := rgb(color)
	fmt.Fprintf(&p.content, "%s %s %s rg\n", number(red), number(green), number(blue))
	p.ellipsePath(cx, cy, rx, ry)
	p.content.WriteString("f\n")
}

// ellipsePath walks the four bezier arcs every renderer draws an ellipse with —
// a PDF has no ellipse of its own.
func (p *Page) ellipsePath(cx, cy, rx, ry Point) {
	const kappa = 0.5523
	y := p.flip(cy)
	fmt.Fprintf(&p.content, "%s %s m\n", number(cx-rx), number(y))
	fmt.Fprintf(&p.content, "%s %s %s %s %s %s c\n",
		number(cx-rx), number(y+ry*kappa), number(cx-rx*kappa), number(y+ry), number(cx), number(y+ry))
	fmt.Fprintf(&p.content, "%s %s %s %s %s %s c\n",
		number(cx+rx*kappa), number(y+ry), number(cx+rx), number(y+ry*kappa), number(cx+rx), number(y))
	fmt.Fprintf(&p.content, "%s %s %s %s %s %s c\n",
		number(cx+rx), number(y-ry*kappa), number(cx+rx*kappa), number(y-ry), number(cx), number(y-ry))
	fmt.Fprintf(&p.content, "%s %s %s %s %s %s c\n",
		number(cx-rx*kappa), number(y-ry), number(cx-rx), number(y-ry*kappa), number(cx-rx), number(y))
}

// Polygon fills a closed shape.
func (p *Page) Polygon(points []Position, color string) {
	if len(points) < 3 {
		return
	}
	red, green, blue := rgb(color)
	fmt.Fprintf(&p.content, "%s %s %s rg\n%s %s m\n", number(red), number(green), number(blue),
		number(points[0].X), number(p.flip(points[0].Y)))
	for _, point := range points[1:] {
		fmt.Fprintf(&p.content, "%s %s l\n", number(point.X), number(p.flip(point.Y)))
	}
	p.content.WriteString("h f\n")
}

// Polyline strokes an open line.
func (p *Page) Polyline(points []Position, color string, width Point) {
	if len(points) < 2 {
		return
	}
	red, green, blue := rgb(color)
	fmt.Fprintf(&p.content, "%s %s %s RG %s w 1 J 1 j\n%s %s m\n",
		number(red), number(green), number(blue), number(width),
		number(points[0].X), number(p.flip(points[0].Y)))
	for _, point := range points[1:] {
		fmt.Fprintf(&p.content, "%s %s l\n", number(point.X), number(p.flip(point.Y)))
	}
	p.content.WriteString("S\n")
}

// Pages is how many pages the document holds.
func (d *Document) Pages() int { return len(d.pages) }

// shading is a wash across one shape, kept until the file is written because a
// PDF states it as a resource of the page rather than inside the drawing.
type shading struct {
	name           string
	from, to       string
	x1, y1, x2, y2 Point
}

// RectShaded fills a rectangle with a wash. The ends are fractions of the
// rectangle's own box, the way the drawing states them.
func (p *Page) RectShaded(x, y, width, height Point, from, to string, x1, y1, x2, y2 Point) {
	if width <= 0 || height <= 0 {
		return
	}
	name := p.shade(from, to, x+x1*width, y+y1*height, x+x2*width, y+y2*height)
	fmt.Fprintf(&p.content, "q %s %s %s %s re W n /%s sh Q\n",
		number(x), number(p.flip(y+height)), number(width), number(height), name)
}

// EllipseShaded fills an ellipse with a wash.
func (p *Page) EllipseShaded(cx, cy, rx, ry Point, from, to string, x1, y1, x2, y2 Point) {
	if rx <= 0 || ry <= 0 {
		return
	}
	name := p.shade(from, to, cx-rx+x1*rx*2, cy-ry+y1*ry*2, cx-rx+x2*rx*2, cy-ry+y2*ry*2)
	p.content.WriteString("q\n")
	p.ellipsePath(cx, cy, rx, ry)
	fmt.Fprintf(&p.content, "W n /%s sh Q\n", name)
}

func (p *Page) shade(from, to string, x1, y1, x2, y2 Point) string {
	name := fmt.Sprintf("Sh%d", len(p.document.shadings)+1)
	p.document.shadings = append(p.document.shadings, shading{name: name, from: from, to: to,
		x1: x1, y1: p.flip(y1), x2: x2, y2: p.flip(y2)})
	p.shadings = append(p.shadings, name)
	return name
}

// WrapText breaks a paragraph into the lines it draws as, at a width and size.
// It measures the face the page is set in rather than guessing from character
// counts, which is how a Korean line and a Latin one can be measured the same
// way.
func (d *Document) WrapText(text string, size, width Point) []string {
	var lines []string
	for _, paragraph := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(paragraph) == "" {
			lines = append(lines, "")
			continue
		}
		current := ""
		for _, character := range paragraph {
			candidate := current + string(character)
			if d.TextWidth(candidate, size) <= width || current == "" {
				current = candidate
				continue
			}
			// Break at the last space where there is one: a line broken mid-word
			// reads as a fault in Latin and is normal in Korean, so the rule is
			// "prefer a space, and break anywhere rather than overflow".
			if at := strings.LastIndex(current, " "); at > 0 {
				lines = append(lines, current[:at])
				current = strings.TrimPrefix(current[at+1:], " ") + string(character)
				continue
			}
			lines = append(lines, current)
			current = string(character)
		}
		lines = append(lines, current)
	}
	return lines
}
