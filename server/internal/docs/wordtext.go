package docs

import (
	"bytes"
	"encoding/xml"
	"strings"
)

// Word does not keep a sentence in one place.
//
// The plain part of a paragraph is runs directly under it, but a link is a
// w:hyperlink holding the runs it covers, a sentence somebody typed with
// revision marks on is a w:ins holding them, and a field, a smart tag or a
// content control wraps them again. Reading only the runs directly under the
// paragraph dropped every word inside those: "자세한 내용은 여기를 참고하십시오"
// arrived as "자세한 내용은 를 참고하십시오", and a paragraph written entirely
// inside one wrapper arrived as nothing at all — no bullet, no warning.

// Nor does it keep every space as a space.
//
// A line break, a carriage return and a tab are elements of their own, not
// characters inside a w:t, so a paragraph broken with Shift+Enter arrived with
// the two lines run together — "첫째 줄둘째 줄" — and a heading somebody numbered
// by hand arrived as "1.개요". A non-breaking hyphen is an element too, so
// "비-대면" arrived as "비대면": a word the document does not contain.

// The namespace an equation writes its text in. It spells the element the same
// way a paragraph does, and a formula is not a sentence, so it is passed over
// here as it was before.
const (
	mathPrefix    = "m"
	mathNamespace = "http://schemas.openxmlformats.org/officeDocument/2006/math"
)

// A paragraph's properties hold a w:tab for every tab stop it defines, which is
// where the tabs line up rather than a tab somebody typed. Neither properties
// element holds any text, so both are passed over whole.
var wordProperties = map[string]bool{"pPr": true, "rPr": true}

// What separates the words on either side of it. A point on a slide is one
// line, so a break between two lines is the space between them.
var wordSeparators = map[string]bool{"br": true, "cr": true, "tab": true}

// The characters Word writes as elements rather than as text.
var wordCharacters = map[string]string{"noBreakHyphen": "-"}

// textIn reads a paragraph's text out of its own XML, in the order it is
// written, however deep the wrappers around it go.
//
// Deleted text is w:delText and a field's instructions are w:instrText, so
// neither is a w:t and neither is read: a paragraph with revision marks still
// reads as what it says now.
func textIn(inner []byte) string {
	decoder := xml.NewDecoder(bytes.NewReader(inner))
	var text strings.Builder
	// depth is how deep inside a text element the reader is, and zero when it
	// is outside one. A w:t holds no elements, but counting rather than
	// flagging means nothing inside an unexpected one is read as a sentence.
	depth := 0
	// skipping is the same count for a properties element, whose contents are
	// how the paragraph is drawn rather than what it says.
	skipping := 0
	// A break is written when the next words arrive, so that a paragraph
	// ending in one does not end in a space, and so that a space already on
	// either side of it is the one space between them.
	broken := false
	write := func(value string) {
		if broken {
			broken = false
			if text.Len() > 0 && !endsInSpace(text.String()) && !startsWithSpace(value) {
				text.WriteByte(' ')
			}
		}
		text.WriteString(value)
	}
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		switch shape := token.(type) {
		case xml.StartElement:
			if skipping > 0 {
				skipping++
				continue
			}
			if depth > 0 {
				depth++
				continue
			}
			switch name := shape.Name.Local; {
			case wordProperties[name]:
				skipping = 1
			case name == "t" && !isEquation(shape.Name.Space):
				depth = 1
			case wordSeparators[name]:
				broken = true
			case wordCharacters[name] != "":
				write(wordCharacters[name])
			}
		case xml.EndElement:
			switch {
			case skipping > 0:
				skipping--
			case depth > 0:
				depth--
			}
		case xml.CharData:
			if depth > 0 {
				write(string(shape))
			}
		}
	}
	return text.String()
}

// The space either side of a break already has, which the break does not add
// to: a run ending in one and a break after it are one space, not two.
const wordSpace = " \t\r\n"

func endsInSpace(value string) bool { return value != strings.TrimRight(value, wordSpace) }

func startsWithSpace(value string) bool { return value != strings.TrimLeft(value, wordSpace) }

// isEquation says whether a namespace is the one equations are written in. The
// paragraph's own XML is read on its own, so a prefix is all there is to go on
// where the file declared no namespace of its own.
func isEquation(space string) bool {
	return space == mathPrefix || space == mathNamespace
}
