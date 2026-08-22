// Package golden compares a whole rendered file against one recorded earlier.
//
// The rest of the suite checks what a deck says: this text overflowed, that
// figure lost its source. A file can be wrong in ways no measurement notices —
// a part declared in one place and missing from another, a relationship id that
// points at nothing, a layout that stops being inherited — and PowerPoint tells
// the author about it, not us. So the outline below is deliberately dumb: every
// element, every attribute, in order. When a change moves something, the diff
// says exactly what moved, and a person decides whether that was the point.
package golden

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/hkjang/ptium/server/internal/pptx"
)

// namespaces are written back as the prefixes the format itself uses, since
// that is how anyone reading OOXML recognises an element.
var namespaces = map[string]string{
	"http://schemas.openxmlformats.org/drawingml/2006/main":                     "a",
	"http://schemas.openxmlformats.org/drawingml/2006/chart":                    "c",
	"http://schemas.openxmlformats.org/presentationml/2006/main":                "p",
	"http://schemas.openxmlformats.org/officeDocument/2006/relationships":       "r",
	"http://schemas.openxmlformats.org/package/2006/relationships":              "rel",
	"http://schemas.openxmlformats.org/package/2006/content-types":              "ct",
	"http://schemas.openxmlformats.org/officeDocument/2006/extended-properties": "ep",
	"http://purl.org/dc/elements/1.1/":                                          "dc",
	"http://purl.org/dc/terms/":                                                 "dcterms",
	"http://schemas.openxmlformats.org/markup-compatibility/2006":               "mc",
	"http://schemas.microsoft.com/office/powerpoint/2010/main":                  "p14",
	"http://www.w3.org/2001/XMLSchema-instance":                                 "xsi",
	"http://www.w3.org/XML/1998/namespace":                                      "xml",
}

// timestamps vary between two runs of the same input, so they are recorded as
// the fact that a time was written rather than as the time.
var timestamp = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z?`)

// Outline writes the whole package as one text: the part list, then each part
// worth reading as an indented tree.
func Outline(data []byte) (string, error) {
	pkg, err := pptx.Open(data)
	if err != nil {
		return "", err
	}
	names := pkg.Names()
	sort.Strings(names)

	var out strings.Builder
	out.WriteString("== parts ==\n")
	for _, name := range names {
		part, _ := pkg.Part(name)
		out.WriteString(fmt.Sprintf("%s  (%s)\n", name, sizeOf(name, part)))
	}
	for _, name := range names {
		if !readable(name) {
			continue
		}
		part, _ := pkg.Part(name)
		out.WriteString("\n== " + name + " ==\n")
		tree, err := outlinePart(part)
		if err != nil {
			return "", fmt.Errorf("%s: %w", name, err)
		}
		out.WriteString(tree)
	}
	return out.String(), nil
}

// sizeOf says how big a part is, except where the size is the point of the
// comparison and would only record the compressor's mood. A picture that
// changes length is a real change; an xml part is read in full below.
func sizeOf(name string, part []byte) string {
	if strings.HasSuffix(name, ".xml") || strings.HasSuffix(name, ".rels") {
		return "xml"
	}
	return fmt.Sprintf("%d bytes", len(part))
}

// readable says whether a part is read as a tree. Media and the embedded
// workbooks behind charts are binary; everything else in a deck is XML we wrote
// and therefore XML we can be wrong about.
func readable(name string) bool {
	return strings.HasSuffix(name, ".xml") || strings.HasSuffix(name, ".rels")
}

func outlinePart(part []byte) (string, error) {
	decoder := xml.NewDecoder(strings.NewReader(string(part)))
	var out strings.Builder
	depth := 0
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		switch node := token.(type) {
		case xml.StartElement:
			out.WriteString(strings.Repeat("  ", depth) + qualify(node.Name))
			if attributes := attributesOf(node); attributes != "" {
				out.WriteString(" " + attributes)
			}
			out.WriteString("\n")
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			text := strings.TrimSpace(string(node))
			if text == "" {
				continue
			}
			out.WriteString(strings.Repeat("  ", depth) + "\"" + normalize(text) + "\"\n")
		}
	}
	return out.String(), nil
}

func qualify(name xml.Name) string {
	if prefix, ok := namespaces[name.Space]; ok {
		return prefix + ":" + name.Local
	}
	if name.Space == "" {
		return name.Local
	}
	return name.Space + ":" + name.Local
}

// attributesOf writes a shape's attributes in a fixed order, dropping the
// namespace declarations — they are the same on every element and would bury
// the line that matters.
func attributesOf(node xml.StartElement) string {
	var pairs []string
	for _, attribute := range node.Attr {
		if attribute.Name.Space == "xmlns" || attribute.Name.Local == "xmlns" {
			continue
		}
		pairs = append(pairs, qualify(attribute.Name)+"="+normalize(attribute.Value))
	}
	sort.Strings(pairs)
	return strings.Join(pairs, " ")
}

func normalize(value string) string {
	return timestamp.ReplaceAllString(value, "<time>")
}
