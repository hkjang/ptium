// Package pdftext reads the words out of a PDF.
//
// A report arrives as a PDF more often than as anything else, and Ptium could
// read every other kind of document a company has. The libraries that do this
// were measured against the files this product is for and answered nothing: a
// thirty-five page Korean deck came back empty, and what did come back was
// mojibake. Korean lives in composite fonts whose codes mean nothing without
// the /ToUnicode map beside them, and a reader that ignores that map produces
// confident nonsense — which is worse than admitting it cannot read.
//
// So this reads what it can prove and says nothing about the rest: pages, their
// content streams, the fonts those streams switch between, and the map from a
// font's own codes to the characters a person would type.
package pdftext

import (
	"bytes"
	"errors"
	"strconv"
)

// value is one PDF object. A PDF has eight kinds and this needs six of them.
type value interface{}

type name string

type ref struct{ number, generation int }

type dict map[name]value

type stream struct {
	dict dict
	raw  []byte
}

type array []value

// lexer walks the bytes of a PDF, which is a text format with binary streams in
// it rather than a binary format with text in it.
type lexer struct {
	data []byte
	at   int
}

func (l *lexer) skipSpace() {
	for l.at < len(l.data) {
		switch l.data[l.at] {
		case ' ', '\t', '\r', '\n', '\f', 0:
			l.at++
		case '%':
			for l.at < len(l.data) && l.data[l.at] != '\n' && l.data[l.at] != '\r' {
				l.at++
			}
		default:
			return
		}
	}
}

func isDelimiter(character byte) bool {
	switch character {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

func isSpace(character byte) bool {
	switch character {
	case ' ', '\t', '\r', '\n', '\f', 0:
		return true
	}
	return false
}

// token reads the next bare word: a number, a keyword, or an operator.
func (l *lexer) token() string {
	l.skipSpace()
	start := l.at
	for l.at < len(l.data) && !isSpace(l.data[l.at]) && !isDelimiter(l.data[l.at]) {
		l.at++
	}
	if start == l.at && l.at < len(l.data) {
		l.at++
		return string(l.data[start:l.at])
	}
	return string(l.data[start:l.at])
}

func (l *lexer) peek() byte {
	l.skipSpace()
	if l.at >= len(l.data) {
		return 0
	}
	return l.data[l.at]
}

var errEnd = errors.New("end of the file")

// object reads one object at the cursor.
func (l *lexer) object() (value, error) {
	l.skipSpace()
	if l.at >= len(l.data) {
		return nil, errEnd
	}
	switch character := l.data[l.at]; {
	case character == '/':
		l.at++
		return l.name(), nil
	case character == '(':
		l.at++
		return l.literalString(), nil
	case character == '<':
		if l.at+1 < len(l.data) && l.data[l.at+1] == '<' {
			l.at += 2
			return l.dictionary()
		}
		l.at++
		return l.hexString(), nil
	case character == '[':
		l.at++
		return l.array()
	case character == ']' || character == '>' || character == '}':
		l.at++
		return nil, nil
	default:
		return l.numberOrKeyword()
	}
}

func (l *lexer) name() name {
	start := l.at
	for l.at < len(l.data) && !isSpace(l.data[l.at]) && !isDelimiter(l.data[l.at]) {
		l.at++
	}
	raw := l.data[start:l.at]
	// A name may spell an awkward character as #XX.
	if !bytes.ContainsRune(raw, '#') {
		return name(raw)
	}
	var said []byte
	for index := 0; index < len(raw); index++ {
		if raw[index] == '#' && index+2 < len(raw) {
			if code, err := strconv.ParseUint(string(raw[index+1:index+3]), 16, 8); err == nil {
				said = append(said, byte(code))
				index += 2
				continue
			}
		}
		said = append(said, raw[index])
	}
	return name(said)
}

// literalString reads (text), which nests parentheses and takes backslash
// escapes — including the octal ones a font's own codes arrive as.
func (l *lexer) literalString() []byte {
	var said []byte
	depth := 1
	for l.at < len(l.data) {
		character := l.data[l.at]
		l.at++
		switch character {
		case '\\':
			if l.at >= len(l.data) {
				return said
			}
			escape := l.data[l.at]
			l.at++
			switch escape {
			case 'n':
				said = append(said, '\n')
			case 'r':
				said = append(said, '\r')
			case 't':
				said = append(said, '\t')
			case 'b':
				said = append(said, '\b')
			case 'f':
				said = append(said, '\f')
			case '\n':
			case '\r':
				if l.at < len(l.data) && l.data[l.at] == '\n' {
					l.at++
				}
			default:
				if escape >= '0' && escape <= '7' {
					code := int(escape - '0')
					for count := 0; count < 2 && l.at < len(l.data) && l.data[l.at] >= '0' && l.data[l.at] <= '7'; count++ {
						code = code*8 + int(l.data[l.at]-'0')
						l.at++
					}
					said = append(said, byte(code))
					continue
				}
				said = append(said, escape)
			}
		case '(':
			depth++
			said = append(said, character)
		case ')':
			depth--
			if depth == 0 {
				return said
			}
			said = append(said, character)
		default:
			said = append(said, character)
		}
	}
	return said
}

func (l *lexer) hexString() []byte {
	var digits []byte
	for l.at < len(l.data) && l.data[l.at] != '>' {
		character := l.data[l.at]
		l.at++
		if isSpace(character) {
			continue
		}
		digits = append(digits, character)
	}
	if l.at < len(l.data) {
		l.at++
	}
	if len(digits)%2 == 1 {
		digits = append(digits, '0')
	}
	said := make([]byte, 0, len(digits)/2)
	for index := 0; index+1 < len(digits); index += 2 {
		code, err := strconv.ParseUint(string(digits[index:index+2]), 16, 8)
		if err != nil {
			continue
		}
		said = append(said, byte(code))
	}
	return said
}

func (l *lexer) array() (array, error) {
	items := array{}
	for {
		l.skipSpace()
		if l.at >= len(l.data) {
			return items, nil
		}
		if l.data[l.at] == ']' {
			l.at++
			return items, nil
		}
		item, err := l.object()
		if err != nil {
			return items, err
		}
		items = append(items, item)
	}
}

func (l *lexer) dictionary() (dict, error) {
	entries := dict{}
	for {
		l.skipSpace()
		if l.at >= len(l.data) {
			return entries, nil
		}
		if l.data[l.at] == '>' {
			l.at++
			if l.at < len(l.data) && l.data[l.at] == '>' {
				l.at++
			}
			return entries, nil
		}
		if l.data[l.at] != '/' {
			// Something is wrong with this dictionary; take what is there.
			if _, err := l.object(); err != nil {
				return entries, err
			}
			continue
		}
		l.at++
		key := l.name()
		item, err := l.object()
		if err != nil {
			return entries, err
		}
		entries[key] = item
	}
}

// numberOrKeyword reads a number, a reference — "12 0 R" — or a bare keyword.
func (l *lexer) numberOrKeyword() (value, error) {
	mark := l.at
	word := l.token()
	if word == "" {
		return nil, errEnd
	}
	switch word {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null":
		return nil, nil
	}
	number, err := strconv.ParseFloat(word, 64)
	if err != nil {
		return keyword(word), nil
	}
	// A reference is two integers and an R, and nothing else looks like that.
	if whole := number == float64(int(number)); whole && number >= 0 {
		save := l.at
		second := l.token()
		if generation, err := strconv.Atoi(second); err == nil {
			if third := l.token(); third == "R" {
				return ref{number: int(number), generation: generation}, nil
			}
		}
		l.at = save
		_ = mark
	}
	return number, nil
}

type keyword string
