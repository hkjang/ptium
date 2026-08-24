// Package pdf writes PDF files: enough of the format to put a deck on paper,
// and no more.
//
// A PDF that cannot draw Korean is not a PDF of a Korean deck, so the text is
// written as glyphs of an embedded font rather than as bytes of a standard
// one — there is no standard PDF font with Hangul in it. That means reading the
// font: which glyph draws which character, and how wide each one is.
package pdf

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// TrueType is the part of a font file this package needs: the tables that say
// what to draw and how much room it takes.
type TrueType struct {
	Data        []byte
	UnitsPerEm  int
	Ascent      int
	Descent     int
	CapHeight   int
	BBox        [4]int
	ItalicAngle int
	// glyphs maps a rune to the glyph that draws it.
	glyphs map[rune]uint16
	// widths is every glyph's advance, in font units.
	widths         []uint16
	longHorMetrics int
}

// ParseTrueType reads a font file. It is strict: a font that cannot be read is
// an error at startup rather than a page of blank paper later.
func ParseTrueType(data []byte) (*TrueType, error) {
	if len(data) < 12 {
		return nil, errors.New("font file is too short")
	}
	switch binary.BigEndian.Uint32(data) {
	case 0x00010000, 0x74727565: // TrueType outlines
	default:
		return nil, fmt.Errorf("unsupported font format %#x", binary.BigEndian.Uint32(data))
	}
	tables := map[string][]byte{}
	count := int(binary.BigEndian.Uint16(data[4:]))
	for index := range count {
		entry := 12 + index*16
		if entry+16 > len(data) {
			return nil, errors.New("font table directory is truncated")
		}
		name := string(data[entry : entry+4])
		offset := int(binary.BigEndian.Uint32(data[entry+8:]))
		length := int(binary.BigEndian.Uint32(data[entry+12:]))
		if offset < 0 || length < 0 || offset+length > len(data) {
			return nil, fmt.Errorf("font table %q lies outside the file", name)
		}
		tables[name] = data[offset : offset+length]
	}
	font := &TrueType{Data: data, glyphs: map[rune]uint16{}}
	head, ok := tables["head"]
	if !ok || len(head) < 54 {
		return nil, errors.New("font has no head table")
	}
	font.UnitsPerEm = int(binary.BigEndian.Uint16(head[18:]))
	if font.UnitsPerEm == 0 {
		font.UnitsPerEm = 1000
	}
	for index, at := range []int{36, 38, 40, 42} {
		font.BBox[index] = int(int16(binary.BigEndian.Uint16(head[at:])))
	}
	if hhea, ok := tables["hhea"]; ok && len(hhea) >= 36 {
		font.Ascent = int(int16(binary.BigEndian.Uint16(hhea[4:])))
		font.Descent = int(int16(binary.BigEndian.Uint16(hhea[6:])))
		font.longHorMetrics = int(binary.BigEndian.Uint16(hhea[34:]))
	}
	if os2, ok := tables["OS/2"]; ok && len(os2) >= 90 {
		font.CapHeight = int(int16(binary.BigEndian.Uint16(os2[88:])))
	}
	if font.CapHeight == 0 {
		font.CapHeight = font.Ascent * 7 / 10
	}
	if post, ok := tables["post"]; ok && len(post) >= 8 {
		// A fixed-point 16.16 angle; the sign is all a PDF needs from it.
		font.ItalicAngle = int(int32(binary.BigEndian.Uint32(post[4:])) >> 16)
	}
	if err := font.readWidths(tables["hmtx"]); err != nil {
		return nil, err
	}
	if err := font.readCmap(tables["cmap"]); err != nil {
		return nil, err
	}
	return font, nil
}

// Glyph is the glyph that draws a rune, and whether the font has one at all.
// A rune the font cannot draw is left to the caller: dropping it silently is
// how a page ends up missing a word nobody notices until it is printed.
func (f *TrueType) Glyph(r rune) (uint16, bool) {
	glyph, ok := f.glyphs[r]
	return glyph, ok && glyph != 0
}

// Width is a glyph's advance in thousandths of an em, which is what PDF counts
// text in.
func (f *TrueType) Width(glyph uint16) int {
	if len(f.widths) == 0 {
		return 1000
	}
	index := int(glyph)
	if index >= len(f.widths) {
		index = len(f.widths) - 1
	}
	return int(f.widths[index]) * 1000 / f.UnitsPerEm
}

func (f *TrueType) readWidths(hmtx []byte) error {
	if len(hmtx) == 0 || f.longHorMetrics == 0 {
		return errors.New("font has no horizontal metrics")
	}
	f.widths = make([]uint16, 0, f.longHorMetrics)
	for index := 0; index < f.longHorMetrics && index*4+2 <= len(hmtx); index++ {
		f.widths = append(f.widths, binary.BigEndian.Uint16(hmtx[index*4:]))
	}
	if len(f.widths) == 0 {
		return errors.New("font states no widths")
	}
	return nil
}

// readCmap reads the character-to-glyph map, preferring the table that covers
// the most: format 12 where the font has one, format 4 otherwise.
func (f *TrueType) readCmap(cmap []byte) error {
	if len(cmap) < 4 {
		return errors.New("font has no character map")
	}
	count := int(binary.BigEndian.Uint16(cmap[2:]))
	best := -1
	bestScore := -1
	for index := range count {
		entry := 4 + index*8
		if entry+8 > len(cmap) {
			break
		}
		platform := binary.BigEndian.Uint16(cmap[entry:])
		encoding := binary.BigEndian.Uint16(cmap[entry+2:])
		offset := int(binary.BigEndian.Uint32(cmap[entry+4:]))
		if offset+4 > len(cmap) {
			continue
		}
		format := binary.BigEndian.Uint16(cmap[offset:])
		score := 0
		switch {
		case platform == 3 && encoding == 10 && format == 12:
			score = 3
		case platform == 3 && encoding == 1 && format == 4:
			score = 2
		case format == 4 || format == 12:
			score = 1
		}
		if score > bestScore {
			best, bestScore = offset, score
		}
	}
	if best < 0 {
		return errors.New("font has no character map this can read")
	}
	switch binary.BigEndian.Uint16(cmap[best:]) {
	case 4:
		return f.readCmap4(cmap[best:])
	case 12:
		return f.readCmap12(cmap[best:])
	}
	return errors.New("font has no character map this can read")
}

func (f *TrueType) readCmap4(table []byte) error {
	if len(table) < 14 {
		return errors.New("character map is truncated")
	}
	segments := int(binary.BigEndian.Uint16(table[6:])) / 2
	ends, starts := 14, 14+segments*2+2
	deltas, ranges := starts+segments*2, starts+segments*4
	if ranges+segments*2 > len(table) {
		return errors.New("character map is truncated")
	}
	for segment := range segments {
		end := binary.BigEndian.Uint16(table[ends+segment*2:])
		start := binary.BigEndian.Uint16(table[starts+segment*2:])
		delta := binary.BigEndian.Uint16(table[deltas+segment*2:])
		rangeOffset := binary.BigEndian.Uint16(table[ranges+segment*2:])
		if start > end {
			continue
		}
		for code := int(start); code <= int(end) && code != 0xFFFF; code++ {
			var glyph uint16
			if rangeOffset == 0 {
				glyph = uint16(code) + delta
			} else {
				at := ranges + segment*2 + int(rangeOffset) + (code-int(start))*2
				if at+2 > len(table) {
					continue
				}
				glyph = binary.BigEndian.Uint16(table[at:])
				if glyph != 0 {
					glyph += delta
				}
			}
			if glyph != 0 {
				f.glyphs[rune(code)] = glyph
			}
		}
	}
	return nil
}

func (f *TrueType) readCmap12(table []byte) error {
	if len(table) < 16 {
		return errors.New("character map is truncated")
	}
	groups := int(binary.BigEndian.Uint32(table[12:]))
	for group := range groups {
		at := 16 + group*12
		if at+12 > len(table) {
			break
		}
		start := binary.BigEndian.Uint32(table[at:])
		end := binary.BigEndian.Uint32(table[at+4:])
		glyph := binary.BigEndian.Uint32(table[at+8:])
		if end < start || end-start > 0x10000 {
			continue
		}
		for code := start; code <= end; code++ {
			f.glyphs[rune(code)] = uint16(glyph + (code - start))
		}
	}
	return nil
}
