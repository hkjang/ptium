package docs

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// The Word reader takes words and tables. A picture in the file is not read,
// and a deck coming back without the photograph somebody put in their report,
// with nothing said about it, is worse than one that says so — the presentation
// reader has said it in these words all along.
func TestAWordDocumentSaysWhatItLeftBehind(t *testing.T) {
	for _, one := range []struct {
		what     string
		body     string
		expected bool
	}{
		{"a report with two photographs", `<w:p><w:r><w:t>본문입니다</w:t></w:r></w:p>` +
			`<w:p><w:r><w:drawing><w:inline/></w:drawing></w:r></w:p>` +
			`<w:p><w:r><w:drawing><w:inline/></w:drawing></w:r></w:p>`, true},
		// Older files and some exporters still write the shape, not the drawing.
		{"a report with an older shape", `<w:p><w:r><w:t>본문입니다</w:t></w:r></w:p>` +
			`<w:p><w:r><w:pict><v:shape/></w:pict></w:r></w:p>`, true},
		{"a report of words alone", `<w:p><w:r><w:t>본문입니다</w:t></w:r></w:p>`, false},
	} {
		document, err := readWordDocument("보고.docx", wordFile(t, one.body))
		if err != nil {
			t.Fatalf("%s: %v", one.what, err)
		}
		said := false
		for _, warning := range document.Warnings {
			if strings.Contains(warning, "그림") {
				said = true
			}
		}
		if said != one.expected {
			t.Errorf("%s: said something about pictures = %v, want %v (%v)",
				one.what, said, one.expected, document.Warnings)
		}
	}
}

// wordFile writes the smallest .docx that carries the given body.
func wordFile(t *testing.T, body string) []byte {
	t.Helper()
	var out bytes.Buffer
	archive := zip.NewWriter(&out)
	part, err := archive.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(`<?xml version="1.0"?><w:document ` +
		`xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" ` +
		`xmlns:v="urn:schemas-microsoft-com:vml"><w:body>` + body + `</w:body></w:document>`)); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
