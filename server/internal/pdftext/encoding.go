package pdftext

import (
	"strconv"
	"strings"
)

// What a simple font's byte codes stand for, when the font says so itself.
//
// A font carries an /Encoding: a base set of glyph names, and often a
// /Differences list that renumbers some of them. Reading past it and printing
// the byte as if it were ASCII is how "매출 이" came out as "매출$이" — the
// subset font had renumbered its glyphs, its space landed on 0x24, and 0x24
// printed as the dollar sign nobody had typed.

// baseGlyphs is the name each code carries in one of the encodings a PDF names.
// The three agree across the letters and digits and part company above them.
func baseGlyphs(encoding name) map[byte]string {
	table := map[byte]string{}
	for code := byte('!'); code <= '~'; code++ {
		if glyph, ok := asciiGlyphs[code]; ok {
			table[code] = glyph
		}
	}
	table[' '] = "space"
	switch encoding {
	case "WinAnsiEncoding":
		for code, glyph := range winAnsiHigh {
			table[code] = glyph
		}
	case "MacRomanEncoding":
		for code, glyph := range macRomanHigh {
			table[code] = glyph
		}
	case "StandardEncoding", "":
		for code, glyph := range standardHigh {
			table[code] = glyph
		}
	}
	return table
}

// readEncoding builds what each byte of a simple font draws.
//
// An /Encoding may be a bare name, or a dictionary naming a base and listing
// the codes that differ from it. A file that lists differences and nothing else
// means the standard set underneath.
func (d *document) readEncoding(entries dict) map[byte]string {
	switch found := d.resolve(entries["Encoding"]).(type) {
	case name:
		return textOfGlyphs(baseGlyphs(found))
	case dict:
		base := name("")
		if named := d.name(found["BaseEncoding"]); named != "" {
			base = named
		}
		glyphs := baseGlyphs(base)
		code := 0
		for _, item := range d.array(found["Differences"]) {
			switch value := d.resolve(item).(type) {
			case float64:
				code = int(value)
			case name:
				if code >= 0 && code <= 255 {
					glyphs[byte(code)] = string(value)
				}
				code++
			}
		}
		return textOfGlyphs(glyphs)
	}
	return nil
}

// textOfGlyphs turns glyph names into the characters a person would read,
// dropping the ones this cannot name rather than inventing a letter for them.
func textOfGlyphs(glyphs map[byte]string) map[byte]string {
	said := make(map[byte]string, len(glyphs))
	for code, glyph := range glyphs {
		if text, ok := glyphText(glyph); ok {
			said[code] = text
		}
	}
	return said
}

// glyphText reads what a glyph name stands for.
//
// Beyond the names every encoding shares, a font may spell a character out as
// uni0AC0 or u1F600, which is a name this can answer from arithmetic. A name
// like g43 or cid7 is the font's own numbering and stands for nothing here.
func glyphText(glyph string) (string, bool) {
	if glyph == "" || glyph == ".notdef" {
		return "", false
	}
	if text, ok := glyphNames[glyph]; ok {
		return text, true
	}
	// A letter names itself: the glyph for A is called "A".
	if len(glyph) == 1 && (glyph[0] >= 'A' && glyph[0] <= 'Z' || glyph[0] >= 'a' && glyph[0] <= 'z') {
		return glyph, true
	}
	if rest, ok := strings.CutPrefix(glyph, "uni"); ok && len(rest) >= 4 {
		if value, err := strconv.ParseUint(rest[:4], 16, 32); err == nil {
			return string(rune(value)), true
		}
	}
	if rest, ok := strings.CutPrefix(glyph, "u"); ok && len(rest) >= 4 && len(rest) <= 6 {
		if value, err := strconv.ParseUint(rest, 16, 32); err == nil {
			return string(rune(value)), true
		}
	}
	// A name with a suffix — "a.sc", "one.oldstyle" — draws the character its
	// stem names.
	if stem, _, found := strings.Cut(glyph, "."); found && stem != "" {
		// The stem carries no suffix of its own, so this settles at once.
		return glyphText(stem)
	}
	return "", false
}

// asciiGlyphs names the codes every encoding agrees about.
var asciiGlyphs = map[byte]string{
	'!': "exclam", '"': "quotedbl", '#': "numbersign", '$': "dollar", '%': "percent",
	'&': "ampersand", '\'': "quotesingle", '(': "parenleft", ')': "parenright",
	'*': "asterisk", '+': "plus", ',': "comma", '-': "hyphen", '.': "period",
	'/': "slash", '0': "zero", '1': "one", '2': "two", '3': "three", '4': "four",
	'5': "five", '6': "six", '7': "seven", '8': "eight", '9': "nine",
	':': "colon", ';': "semicolon", '<': "less", '=': "equal", '>': "greater",
	'?': "question", '@': "at", '[': "bracketleft", '\\': "backslash",
	']': "bracketright", '^': "asciicircum", '_': "underscore", '`': "grave",
	'{': "braceleft", '|': "bar", '}': "braceright", '~': "asciitilde",
}

func init() {
	for letter := byte('A'); letter <= 'Z'; letter++ {
		asciiGlyphs[letter] = string(rune(letter))
		asciiGlyphs[letter+32] = string(rune(letter + 32))
	}
}

// glyphNames is what each name stands for. The letters and digits name
// themselves; the rest are the names the three encodings use.
var glyphNames = map[string]string{
	"space": " ", "exclam": "!", "quotedbl": "\"", "numbersign": "#", "dollar": "$",
	"percent": "%", "ampersand": "&", "quotesingle": "'", "quoteright": "’",
	"quoteleft": "‘", "parenleft": "(", "parenright": ")", "asterisk": "*",
	"plus": "+", "comma": ",", "hyphen": "-", "period": ".", "slash": "/",
	"zero": "0", "one": "1", "two": "2", "three": "3", "four": "4", "five": "5",
	"six": "6", "seven": "7", "eight": "8", "nine": "9", "colon": ":",
	"semicolon": ";", "less": "<", "equal": "=", "greater": ">", "question": "?",
	"at": "@", "bracketleft": "[", "backslash": "\\", "bracketright": "]",
	"asciicircum": "^", "underscore": "_", "grave": "`", "braceleft": "{",
	"bar": "|", "braceright": "}", "asciitilde": "~",
	// The marks a report is set with.
	"quotedblleft": "“", "quotedblright": "”", "quotedblbase": "„",
	"quotesinglbase": "‚", "endash": "–", "emdash": "—",
	"bullet": "•", "ellipsis": "…", "dagger": "†",
	"daggerdbl": "‡", "perthousand": "‰", "guilsinglleft": "‹",
	"guilsinglright": "›", "guillemotleft": "«", "guillemotright": "»",
	"fraction": "⁄", "florin": "ƒ", "trademark": "™",
	"Euro": "€", "minus": "−", "fi": "fi", "fl": "fl",
	// Latin-1, which is where an accented name lands.
	"exclamdown": "¡", "cent": "¢", "sterling": "£",
	"currency": "¤", "yen": "¥", "brokenbar": "¦",
	"section": "§", "dieresis": "¨", "copyright": "©",
	"ordfeminine": "ª", "logicalnot": "¬", "registered": "®",
	"macron": "¯", "degree": "°", "plusminus": "±",
	"acute": "´", "mu": "µ", "paragraph": "¶",
	"periodcentered": "·", "cedilla": "¸", "ordmasculine": "º",
	"onequarter": "¼", "onehalf": "½", "threequarters": "¾",
	"questiondown": "¿", "Agrave": "À", "Aacute": "Á",
	"Acircumflex": "Â", "Atilde": "Ã", "Adieresis": "Ä",
	"Aring": "Å", "AE": "Æ", "Ccedilla": "Ç", "Egrave": "È",
	"Eacute": "É", "Ecircumflex": "Ê", "Edieresis": "Ë",
	"Igrave": "Ì", "Iacute": "Í", "Icircumflex": "Î",
	"Idieresis": "Ï", "Eth": "Ð", "Ntilde": "Ñ", "Ograve": "Ò",
	"Oacute": "Ó", "Ocircumflex": "Ô", "Otilde": "Õ",
	"Odieresis": "Ö", "multiply": "×", "Oslash": "Ø",
	"Ugrave": "Ù", "Uacute": "Ú", "Ucircumflex": "Û",
	"Udieresis": "Ü", "Yacute": "Ý", "Thorn": "Þ",
	"germandbls": "ß", "agrave": "à", "aacute": "á",
	"acircumflex": "â", "atilde": "ã", "adieresis": "ä",
	"aring": "å", "ae": "æ", "ccedilla": "ç", "egrave": "è",
	"eacute": "é", "ecircumflex": "ê", "edieresis": "ë",
	"igrave": "ì", "iacute": "í", "icircumflex": "î",
	"idieresis": "ï", "eth": "ð", "ntilde": "ñ", "ograve": "ò",
	"oacute": "ó", "ocircumflex": "ô", "otilde": "õ",
	"odieresis": "ö", "divide": "÷", "oslash": "ø",
	"ugrave": "ù", "uacute": "ú", "ucircumflex": "û",
	"udieresis": "ü", "yacute": "ý", "thorn": "þ",
	"ydieresis": "ÿ", "Scaron": "Š", "scaron": "š",
	"Zcaron": "Ž", "zcaron": "ž", "Ydieresis": "Ÿ",
	"OE": "Œ", "oe": "œ", "circumflex": "ˆ", "tilde": "˜",
	"breve": "˘", "dotaccent": "˙", "ring": "˚", "ogonek": "˛",
	"hungarumlaut": "˝", "caron": "ˇ", "dotlessi": "ı",
	"Lslash": "Ł", "lslash": "ł",
}

// The names above 126, per encoding. Below that the three agree.
var winAnsiHigh = map[byte]string{
	0x80: "Euro", 0x82: "quotesinglbase", 0x83: "florin", 0x84: "quotedblbase",
	0x85: "ellipsis", 0x86: "dagger", 0x87: "daggerdbl", 0x88: "circumflex",
	0x89: "perthousand", 0x8A: "Scaron", 0x8B: "guilsinglleft", 0x8C: "OE",
	0x8E: "Zcaron", 0x91: "quoteleft", 0x92: "quoteright", 0x93: "quotedblleft",
	0x94: "quotedblright", 0x95: "bullet", 0x96: "endash", 0x97: "emdash",
	0x98: "tilde", 0x99: "trademark", 0x9A: "scaron", 0x9B: "guilsinglright",
	0x9C: "oe", 0x9E: "zcaron", 0x9F: "Ydieresis", 0xA0: "space",
}

var standardHigh = map[byte]string{
	0xA1: "exclamdown", 0xA2: "cent", 0xA3: "sterling", 0xA4: "fraction",
	0xA5: "yen", 0xA6: "florin", 0xA7: "section", 0xA8: "currency",
	0xA9: "quotesingle", 0xAA: "quotedblleft", 0xAB: "guillemotleft",
	0xAC: "guilsinglleft", 0xAD: "guilsinglright", 0xAE: "fi", 0xAF: "fl",
	0xB1: "endash", 0xB2: "dagger", 0xB3: "daggerdbl", 0xB4: "periodcentered",
	0xB6: "paragraph", 0xB7: "bullet", 0xB8: "quotesinglbase",
	0xB9: "quotedblbase", 0xBA: "quotedblright", 0xBB: "guillemotright",
	0xBC: "ellipsis", 0xBD: "perthousand", 0xBF: "questiondown",
	0xC1: "grave", 0xC2: "acute", 0xC3: "circumflex", 0xC4: "tilde",
	0xC5: "macron", 0xC6: "breve", 0xC7: "dotaccent", 0xC8: "dieresis",
	0xCA: "ring", 0xCB: "cedilla", 0xCD: "hungarumlaut", 0xCE: "ogonek",
	0xCF: "caron", 0xD0: "emdash", 0xE1: "AE", 0xE3: "ordfeminine",
	0xE8: "Lslash", 0xE9: "Oslash", 0xEA: "OE", 0xEB: "ordmasculine",
	0xF1: "ae", 0xF5: "dotlessi", 0xF8: "lslash", 0xF9: "oslash",
	0xFA: "oe", 0xFB: "germandbls",
}

var macRomanHigh = map[byte]string{
	0x80: "Adieresis", 0x81: "Aring", 0x82: "Ccedilla", 0x83: "Eacute",
	0x84: "Ntilde", 0x85: "Odieresis", 0x86: "Udieresis", 0x87: "aacute",
	0x88: "agrave", 0x89: "acircumflex", 0x8A: "adieresis", 0x8B: "atilde",
	0x8C: "aring", 0x8D: "ccedilla", 0x8E: "eacute", 0x8F: "egrave",
	0x90: "ecircumflex", 0x91: "edieresis", 0x92: "iacute", 0x93: "igrave",
	0x94: "icircumflex", 0x95: "idieresis", 0x96: "ntilde", 0x97: "oacute",
	0x98: "ograve", 0x99: "ocircumflex", 0x9A: "odieresis", 0x9B: "otilde",
	0x9C: "uacute", 0x9D: "ugrave", 0x9E: "ucircumflex", 0x9F: "udieresis",
	0xA0: "dagger", 0xA1: "degree", 0xA2: "cent", 0xA3: "sterling",
	0xA4: "section", 0xA5: "bullet", 0xA6: "paragraph", 0xA7: "germandbls",
	0xA8: "registered", 0xA9: "copyright", 0xAA: "trademark", 0xAB: "acute",
	0xAC: "dieresis", 0xAE: "AE", 0xAF: "Oslash", 0xB1: "plusminus",
	0xB5: "mu", 0xBA: "ordfeminine", 0xBB: "ordmasculine", 0xBE: "ae",
	0xBF: "oslash", 0xC0: "questiondown", 0xC1: "exclamdown", 0xC2: "logicalnot",
	0xC4: "florin", 0xC7: "guillemotleft", 0xC8: "guillemotright",
	0xC9: "ellipsis", 0xCA: "space", 0xCB: "Agrave", 0xCC: "Atilde",
	0xCD: "Otilde", 0xCE: "OE", 0xCF: "oe", 0xD0: "endash", 0xD1: "emdash",
	0xD2: "quotedblleft", 0xD3: "quotedblright", 0xD4: "quoteleft",
	0xD5: "quoteright", 0xD6: "divide", 0xD8: "ydieresis", 0xD9: "Ydieresis",
	0xDA: "fraction", 0xDB: "currency", 0xDC: "guilsinglleft",
	0xDD: "guilsinglright", 0xDE: "fi", 0xDF: "fl", 0xE0: "daggerdbl",
	0xE1: "periodcentered", 0xE2: "quotesinglbase", 0xE3: "quotedblbase",
	0xE4: "perthousand", 0xE5: "Acircumflex", 0xE6: "Ecircumflex",
	0xE7: "Aacute", 0xE8: "Edieresis", 0xE9: "Egrave", 0xEA: "Iacute",
	0xEB: "Icircumflex", 0xEC: "Idieresis", 0xED: "Igrave", 0xEE: "Oacute",
	0xEF: "Ocircumflex", 0xF1: "Ograve", 0xF2: "Uacute", 0xF3: "Ucircumflex",
	0xF4: "Ugrave", 0xF5: "dotlessi", 0xF6: "circumflex", 0xF7: "tilde",
	0xF8: "macron", 0xF9: "breve", 0xFA: "dotaccent", 0xFB: "ring",
	0xFC: "cedilla", 0xFD: "hungarumlaut", 0xFE: "ogonek", 0xFF: "caron",
}
