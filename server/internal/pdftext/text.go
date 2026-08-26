package pdftext

import (
	"math"
	"strings"
	"unicode/utf16"
)

// Page is what one page of a PDF says, in the order the page says it.
type Page struct {
	Number int
	Lines  []string
}

// Reading is what a PDF turned out to hold.
type Reading struct {
	Pages []Page
	// Short says the file asked to unpack more than this reads in one go, so
	// Pages is the front of the document rather than all of it. The pages past
	// it were not read, which is a different thing from being empty — and
	// calling a document too big to read "a scan" sends somebody to fix the
	// wrong thing.
	Short bool
	// Total is how many pages the file has, read or not.
	Total int
}

// Read takes the words out of a PDF.
//
// A page whose text this cannot prove — a scan, a deck exported as pictures, a
// font with no map from its codes to characters — comes back with no lines
// rather than with guesses. Confident nonsense is worse than an empty answer:
// the import can say "this file has no text in it", and a person can act on
// that.
func Read(data []byte) (Reading, error) {
	doc, err := open(data)
	if err != nil {
		return Reading{}, err
	}
	all := doc.pages()
	read := Reading{Pages: make([]Page, 0, 16), Total: len(all)}
	for index, page := range all {
		fonts := doc.fontsOf(page)
		content := doc.contentOf(page)
		if doc.exhausted {
			// The page the budget ran out on was read in part or not at all.
			// Half a page is not what it says, so it is left out with the rest.
			read.Short = true
			break
		}
		read.Pages = append(read.Pages, Page{Number: index + 1, Lines: extractLines(content, fonts)})
	}
	return read, nil
}

// pages walks the page tree, in reading order.
func (d *document) pages() []dict {
	var root dict
	for number := range d.objects {
		if entries := d.dict(d.objects[number]); entries != nil && d.name(entries["Type"]) == "Catalog" {
			root = d.dict(entries["Pages"])
			break
		}
	}
	var found []dict
	seen := map[int]bool{}
	var walk func(node dict, depth int)
	walk = func(node dict, depth int) {
		if node == nil || depth > 32 {
			return
		}
		switch d.name(node["Type"]) {
		case "Page":
			found = append(found, node)
			return
		default:
			for _, kid := range d.array(node["Kids"]) {
				if pointer, ok := kid.(ref); ok {
					if seen[pointer.number] {
						continue
					}
					seen[pointer.number] = true
				}
				walk(d.dict(kid), depth+1)
			}
		}
	}
	walk(root, 0)
	if len(found) > 0 {
		return found
	}
	// A file whose catalogue this could not follow still has its pages in it.
	for number := 0; number < len(d.objects)+len(d.streams)+8; number++ {
		if entries := d.dict(d.objects[number]); entries != nil && d.name(entries["Type"]) == "Page" {
			found = append(found, entries)
		}
	}
	return found
}

// contentOf is the page's content stream, expanded.
func (d *document) contentOf(page dict) []byte {
	var said []byte
	add := func(item value) {
		pointer, ok := item.(ref)
		if !ok {
			return
		}
		if len(said) >= maximumPageBytes {
			return
		}
		if data, ok := d.decoded(pointer.number); ok {
			said = append(said, data...)
			said = append(said, '\n')
		}
	}
	switch content := page["Contents"].(type) {
	case ref:
		// An array of streams behind one reference is one page's content.
		if items, ok := d.resolve(content).(array); ok {
			for _, item := range items {
				add(item)
			}
			break
		}
		add(content)
	case array:
		for _, item := range content {
			add(item)
		}
	}
	return said
}

// font is what one /Font resource can tell us about the bytes it draws.
type font struct {
	// toUnicode maps a code — one byte for a simple font, two for a composite
	// one — to what a person would type.
	toUnicode map[uint32]string
	// twoByte says the codes in a string are pairs, which is how Korean,
	// Japanese and Chinese are written in a PDF.
	twoByte bool
	// simple says the font's codes are bytes of a Latin encoding, which can be
	// read without a map.
	simple bool
}

func (d *document) fontsOf(page dict) map[name]*font {
	fonts := map[name]*font{}
	resources := d.dict(page["Resources"])
	if resources == nil {
		return fonts
	}
	for key, item := range d.dict(resources["Font"]) {
		entries := d.dict(item)
		if entries == nil {
			continue
		}
		fonts[key] = d.readFont(entries)
	}
	return fonts
}

func (d *document) readFont(entries dict) *font {
	held := &font{}
	subtype := d.name(entries["Subtype"])
	held.twoByte = subtype == "Type0"
	held.simple = !held.twoByte
	if pointer, ok := entries["ToUnicode"].(ref); ok {
		if mapped, ok := d.charted[pointer.number]; ok {
			held.toUnicode = mapped
		} else if data, ok := d.decoded(pointer.number); ok {
			held.toUnicode = parseToUnicode(data)
			d.charted[pointer.number] = held.toUnicode
		}
	}
	// A composite font with no map is a font whose codes mean nothing here.
	if held.twoByte && len(held.toUnicode) == 0 {
		if descendants := d.array(entries["DescendantFonts"]); len(descendants) > 0 {
			if encoding := d.name(entries["Encoding"]); encoding == "Identity-H" {
				held.toUnicode = nil
			}
		}
	}
	return held
}

// parseToUnicode reads the CMap that maps a font's codes to characters.
//
// This is the piece the libraries measured against these files skipped, and it
// is the whole of reading Korean out of a PDF: the codes are the font's own
// glyph numbers and mean nothing without it.
func parseToUnicode(data []byte) map[uint32]string {
	mapped := map[uint32]string{}
	reader := &lexer{data: data}
	var stack []value
	for {
		item, err := reader.object()
		if err != nil {
			break
		}
		word, ok := item.(keyword)
		if !ok {
			// A CMap block is supposed to hold at most a hundred entries and
			// the files people actually have hold twelve thousand. Trimming
			// the front of this stack drops the pairs at the start of the
			// block, which is how a reader ends up with a map that covers a
			// tenth of the page and reads the rest as nothing.
			if len(stack) < maximumCMapEntries {
				stack = append(stack, item)
			}
			continue
		}
		switch word {
		case "endbfchar":
			for index := 0; index+1 < len(stack); index += 2 {
				code, okCode := codeOf(stack[index])
				said, okSaid := textOf(stack[index+1])
				if okCode && okSaid {
					mapped[code] = said
				}
			}
			stack = stack[:0]
		case "endbfrange":
			for index := 0; index+2 < len(stack); index += 3 {
				low, okLow := codeOf(stack[index])
				high, okHigh := codeOf(stack[index+1])
				if !okLow || !okHigh || high < low || high-low > 65535 {
					continue
				}
				switch destination := stack[index+2].(type) {
				case array:
					for offset := uint32(0); offset <= high-low && int(offset) < len(destination); offset++ {
						if said, ok := textOf(destination[offset]); ok {
							mapped[low+offset] = said
						}
					}
				default:
					said, ok := textOf(stack[index+2])
					if !ok || said == "" {
						continue
					}
					runes := []rune(said)
					for offset := uint32(0); offset <= high-low; offset++ {
						next := append([]rune{}, runes...)
						next[len(next)-1] += rune(offset)
						mapped[low+offset] = string(next)
					}
				}
			}
			stack = stack[:0]
		case "beginbfchar", "beginbfrange", "begincidrange", "endcidrange":
			stack = stack[:0]
		}
	}
	return mapped
}

// maximumCMapEntries bounds one bfchar or bfrange block so a malformed file
// cannot make the reader allocate without end. A font holds at most 65,536
// codes, and each entry is at most three values.
const maximumCMapEntries = 3 * 65536

func codeOf(item value) (uint32, bool) {
	raw, ok := item.([]byte)
	if !ok || len(raw) == 0 || len(raw) > 4 {
		return 0, false
	}
	code := uint32(0)
	for _, character := range raw {
		code = code<<8 | uint32(character)
	}
	return code, true
}

// textOf reads the character a code maps to. A CMap writes it as UTF-16BE.
func textOf(item value) (string, bool) {
	raw, ok := item.([]byte)
	if !ok || len(raw) == 0 {
		return "", false
	}
	if len(raw)%2 == 1 {
		raw = append(raw, 0)
	}
	units := make([]uint16, 0, len(raw)/2)
	for index := 0; index+1 < len(raw); index += 2 {
		units = append(units, uint16(raw[index])<<8|uint16(raw[index+1]))
	}
	said := strings.TrimRight(string(utf16.Decode(units)), "\x00")
	return said, said != ""
}

// composeHangul joins the jamo a font's map hands back in pieces.
//
// A Korean subset font maps its codes to conjoining jamo — ᄒ, ᅭ, ᆼ — rather
// than to syllables, and a slide that carries those as separate characters
// reads as gibberish in PowerPoint. The composition is arithmetic, and it is
// the same arithmetic Unicode itself specifies.
func composeHangul(said string) string {
	const (
		leadBase   = 0x1100
		vowelBase  = 0x1161
		tailBase   = 0x11A7
		syllable   = 0xAC00
		vowelCount = 21
		tailCount  = 28
	)
	runes := []rune(said)
	if len(runes) < 2 {
		return said
	}
	joined := make([]rune, 0, len(runes))
	for _, character := range runes {
		if len(joined) > 0 {
			last := joined[len(joined)-1]
			if last >= leadBase && last < leadBase+19 && character >= vowelBase && character < vowelBase+vowelCount {
				joined[len(joined)-1] = rune(syllable + (int(last-leadBase)*vowelCount+int(character-vowelBase))*tailCount)
				continue
			}
			if last >= syllable && last < syllable+11172 && (last-syllable)%tailCount == 0 &&
				character > tailBase && character < tailBase+tailCount {
				joined[len(joined)-1] = last + (character - tailBase)
				continue
			}
		}
		joined = append(joined, character)
	}
	return string(joined)
}

// matrix is the part of a PDF transform that says where on the page something
// is drawn. Only the scale and the offset matter for reading text back.
type matrix struct{ a, b, c, d, e, f float64 }

var identity = matrix{a: 1, d: 1}

// offsetBy is the translation a Td applies to the line matrix.
func (m matrix) offsetBy(x, y float64) matrix {
	return matrix{
		a: m.a, b: m.b, c: m.c, d: m.d,
		e: x*m.a + y*m.c + m.e,
		f: x*m.b + y*m.d + m.f,
	}
}

// through is this matrix followed by another: where text space lands on the
// page once the page's own transform has been applied to it.
func (m matrix) through(outer matrix) matrix {
	return matrix{
		a: m.a*outer.a + m.b*outer.c,
		b: m.a*outer.b + m.b*outer.d,
		c: m.c*outer.a + m.d*outer.c,
		d: m.c*outer.b + m.d*outer.d,
		e: m.e*outer.a + m.f*outer.c + outer.e,
		f: m.e*outer.b + m.f*outer.d + outer.f,
	}
}

// tall is how high one unit of this matrix draws.
func (m matrix) tall() float64 { return math.Hypot(m.c, m.d) }

// extractLines walks a content stream and gathers what it draws, in lines.
//
// A PDF has no lines in it. It has glyphs at coordinates, and a generator is
// free to place every single letter with its own instruction — several of the
// reports this was measured against do exactly that. Treating each drawing
// instruction as a line turns such a page into one letter per bullet, so a line
// here is what shares a baseline, and a gap wide enough to be a space becomes
// one.
func extractLines(content []byte, fonts map[name]*font) []string {
	placed := extractPlaced(content, fonts)
	lines := make([]string, 0, len(placed))
	for _, line := range placed {
		lines = append(lines, line.text)
	}
	return lines
}

// placed is a line and where on the page it was drawn.
type placed struct {
	text string
	x, y float64
}

func extractPlaced(content []byte, fonts map[name]*font) []placed {
	if len(content) == 0 {
		return nil
	}
	reader := &lexer{data: content}
	var stack []value
	var lines []placed
	var line strings.Builder
	var startedAt float64
	var current *font
	text, next := identity, identity
	// A page is free to move, scale and flip everything drawn on it, and the
	// files people have do exactly that — several write "1 0 0 -1 0 595 cm" and
	// then draw upside down into it. Without this, a line's height and the gap
	// before it are measured in a space the page has already changed.
	here := identity
	var stacked []matrix
	size, leading := 10.0, 0.0
	baseline, pen := 0.0, 0.0
	open := false

	endLine := func() {
		// The jamo are joined here rather than as each code is read: a map
		// hands back one jamo per code, and a syllable is three of them.
		if said := composeHangul(strings.Join(strings.Fields(line.String()), " ")); said != "" {
			lines = append(lines, placed{text: said, x: startedAt, y: baseline})
		}
		line.Reset()
		open = false
	}
	// drawn is the height one unit of the font is drawn at, which is what makes
	// "the same baseline" and "a wide gap" mean the same thing on a page scaled
	// by its own matrix as on one that is not.
	drawn := func() float64 {
		height := size * text.through(here).tall()
		if height <= 0 {
			height = 10
		}
		return height
	}
	show := func(item value) {
		raw, ok := item.([]byte)
		if !ok {
			return
		}
		said := decodeShown(raw, current)
		if said == "" {
			return
		}
		height := drawn()
		at := text.through(here)
		switch {
		case !open:
			open = true
			startedAt = at.e
		case difference(at.f, baseline) > height*0.4,
			at.e < pen-height*0.5,
			at.e-pen > height*4:
			// A gap this wide on one baseline is the next column, not a space.
			endLine()
			open = true
			startedAt = at.e
		case at.e-pen > height*0.22:
			line.WriteString(" ")
		}
		line.WriteString(said)
		// The pen moves in the space the text is written in; where that lands
		// on the page is worked out again from the page's own transform.
		text.e += widthOf(said, size)
		baseline, pen = at.f, text.through(here).e
	}
	number := func(at int) float64 {
		if len(stack) < at {
			return 0
		}
		value, _ := stack[len(stack)-at].(float64)
		return value
	}
	for {
		item, err := reader.object()
		if err != nil {
			break
		}
		word, ok := item.(keyword)
		if !ok {
			stack = append(stack, item)
			if len(stack) > 64 {
				stack = stack[len(stack)-32:]
			}
			continue
		}
		switch word {
		case "q":
			stacked = append(stacked, here)
		case "Q":
			if len(stacked) > 0 {
				here = stacked[len(stacked)-1]
				stacked = stacked[:len(stacked)-1]
			}
		case "cm":
			if len(stack) >= 6 {
				applied := matrix{a: number(6), b: number(5), c: number(4), d: number(3), e: number(2), f: number(1)}
				here = applied.through(here)
			}
		case "BT":
			text, next = identity, identity
		case "ET":
			// A line is not ended here. Several generators open and close a
			// text object around every single word, and ending the line at
			// every ET is how "기능 · 역할 및 비즈니스 가치" arrives as six
			// bullets. What ends a line is a change of baseline.
		case "Tf":
			if len(stack) >= 2 {
				if key, ok := stack[len(stack)-2].(name); ok {
					current = fonts[key]
				}
				size = number(1)
			}
		case "TL":
			leading = number(1)
		case "Tm":
			if len(stack) >= 6 {
				next = matrix{a: number(6), b: number(5), c: number(4), d: number(3), e: number(2), f: number(1)}
				text = next
			}
		case "Td":
			next = next.offsetBy(number(2), number(1))
			text = next
		case "TD":
			leading = -number(1)
			next = next.offsetBy(number(2), number(1))
			text = next
		case "T*":
			next = next.offsetBy(0, -leading)
			text = next
		case "Tj":
			if len(stack) > 0 {
				show(stack[len(stack)-1])
			}
		case "'", "\"":
			next = next.offsetBy(0, -leading)
			text = next
			if len(stack) > 0 {
				show(stack[len(stack)-1])
			}
		case "TJ":
			if len(stack) > 0 {
				if items, ok := stack[len(stack)-1].(array); ok {
					for _, part := range items {
						switch shown := part.(type) {
						case []byte:
							show(shown)
						case float64:
							// A kern moves the pen, and a wide one is the space
							// a page draws by moving rather than by writing.
							text.e -= shown / 1000 * size
						}
					}
				}
			}
		}
		if word != "" {
			stack = stack[:0]
		}
	}
	endLine()
	return lines
}

func difference(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}

// widthOf estimates how far along the line a string reaches.
//
// The true answer is in the font's own width table, and reading that table for
// every subset font in a file costs more than what it buys: the estimate is
// only used to decide whether the next glyph starts a new word, and a Latin
// letter is about half its height while a Korean one is about all of it.
func widthOf(said string, height float64) float64 {
	total := 0.0
	for _, character := range said {
		switch {
		case character >= 0x1100 && character <= 0x11FF,
			character >= 0x3000 && character <= 0x9FFF,
			character >= 0xAC00 && character <= 0xD7A3,
			character >= 0xFF01 && character <= 0xFF60:
			total += 1
		case character == ' ':
			total += 0.28
		default:
			total += 0.5
		}
	}
	return total * height
}

// decodeShown turns the bytes of a shown string into what a person would read.
func decodeShown(raw []byte, held *font) string {
	if held != nil && len(held.toUnicode) > 0 {
		var said strings.Builder
		step := 1
		if held.twoByte {
			step = 2
		}
		for index := 0; index+step <= len(raw); index += step {
			code := uint32(raw[index])
			if step == 2 {
				code = code<<8 | uint32(raw[index+1])
			}
			if mapped, ok := held.toUnicode[code]; ok {
				said.WriteString(mapped)
				continue
			}
			if step == 1 && raw[index] >= 32 && raw[index] < 127 {
				said.WriteByte(raw[index])
				continue
			}
			// A code the map does not cover is a glyph this cannot name. It
			// leaves a gap rather than nothing: the codes a subset font leaves
			// out of its map are nearly always the spaces between words, and a
			// slide that says "효과적인팀협업도구소개" is a slide nobody wrote.
			said.WriteByte(' ')
		}
		return said.String()
	}
	if held != nil && held.twoByte {
		// Two-byte codes with no map are glyph numbers. Nothing readable can be
		// made of them, and inventing letters is how a reader lies.
		return ""
	}
	if said, ok := utf16Korean(raw); ok {
		return said
	}
	// A simple font with no map draws bytes that are almost always Latin-1.
	var said strings.Builder
	for _, character := range raw {
		if character >= 32 && character < 127 {
			said.WriteByte(character)
			continue
		}
		if character >= 160 {
			said.WriteRune(rune(character))
		}
	}
	return said.String()
}

// utf16Korean reads a string that a generator wrote as UTF-16BE while telling
// the file it was Latin.
//
// This is not a guess. Latin-1 bytes do not decode into an unbroken run of
// Hangul by accident, and several of the files people bring to this product are
// written exactly this way: the text is real Korean, and the font dictionary
// that describes it is wrong. Refusing to read them would lose the whole page;
// reading the bytes as the font claims would print Ö¨¬üÈÇ.
func utf16Korean(raw []byte) (string, bool) {
	if len(raw) < 2 {
		return "", false
	}
	units := make([]uint16, 0, len(raw))
	hangul := 0
	for index := 0; index < len(raw); {
		// The generators that do this write one byte for a character that fits
		// in one and two for a character that does not, so a space inside
		// Korean text is a single 0x20.
		if raw[index] < 0x80 {
			if raw[index] < 0x09 || (raw[index] > 0x0D && raw[index] < 0x20) {
				return "", false
			}
			units = append(units, uint16(raw[index]))
			index++
			continue
		}
		if index+1 >= len(raw) {
			return "", false
		}
		unit := uint16(raw[index])<<8 | uint16(raw[index+1])
		index += 2
		switch {
		case unit >= 0xAC00 && unit <= 0xD7A3, unit >= 0x3130 && unit <= 0x318F:
			hangul++
		case unit >= 0x3000 && unit <= 0x303F, unit >= 0xFF01 && unit <= 0xFF5E:
		case unit >= 0x4E00 && unit <= 0x9FFF:
		default:
			return "", false
		}
		units = append(units, unit)
	}
	if hangul == 0 {
		return "", false
	}
	return string(utf16.Decode(units)), true
}
