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

// The namespace an equation writes its text in. It spells the element the same
// way a paragraph does, and a formula is not a sentence, so it is passed over
// here as it was before.
const (
	mathPrefix    = "m"
	mathNamespace = "http://schemas.openxmlformats.org/officeDocument/2006/math"
)

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
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		switch shape := token.(type) {
		case xml.StartElement:
			if depth > 0 {
				depth++
				continue
			}
			if shape.Name.Local == "t" && !isEquation(shape.Name.Space) {
				depth = 1
			}
		case xml.EndElement:
			if depth > 0 {
				depth--
			}
		case xml.CharData:
			if depth > 0 {
				text.Write(shape)
			}
		}
	}
	return text.String()
}

// isEquation says whether a namespace is the one equations are written in. The
// paragraph's own XML is read on its own, so a prefix is all there is to go on
// where the file declared no namespace of its own.
func isEquation(space string) bool {
	return space == mathPrefix || space == mathNamespace
}
