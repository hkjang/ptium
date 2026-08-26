package httpapi

import (
	"strings"
	"testing"
)

// A file that says it is a presentation and will not open is a presentation
// with something wrong with it — nearly always document security. Sending it to
// the document reader answered "Ptium reads .pptx presentations and .xlsx,
// .csv, .docx, .pdf and .md documents" about a .pptx, which is false and no
// help: the person holding the file can unlock it in a minute if somebody says
// that is what is wrong.
func TestAFileThatCannotBeOpenedIsNamedForWhatItIs(t *testing.T) {
	locked := append([]byte("SCDS"), make([]byte, 64)...)
	if said := templateUploadHint(locked); said == "" {
		t.Fatal("a document-security wrapper is not recognised")
	} else if !strings.Contains(said, "DRM") {
		t.Errorf("the message does not name it: %q", said)
	}
	old := append([]byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, make([]byte, 64)...)
	if said := templateUploadHint(old); said == "" || !strings.Contains(said, ".pptx") {
		t.Errorf("a 97-2003 file is not told what to save it as: %q", said)
	}
	// An Office package that is not a presentation says what it is in the names
	// of its own parts, and this product reads both — so the answer is which
	// door to use, not "the package does not contain a PowerPoint presentation".
	word := []byte("PK\x03\x04................word/document.xml")
	if said := templateUploadHint(word); !strings.Contains(said, "Word") || !strings.Contains(said, ".docx") {
		t.Errorf("a renamed Word file is not named for what it is: %q", said)
	}
	sheet := []byte("PK\x03\x04................xl/workbook.xml")
	if said := templateUploadHint(sheet); !strings.Contains(said, "Excel") || !strings.Contains(said, ".xlsx") {
		t.Errorf("a renamed Excel file is not named for what it is: %q", said)
	}
	// Anything else that got here is a package that would not open.
	if said := templateUploadHint([]byte("PK\x03\x04 an ordinary zip")); !strings.Contains(said, "열리지 않습니다") {
		t.Errorf("a package that would not open was not told so: %q", said)
	}
	// And every one of these is written in the language the workspace is in.
	for _, data := range [][]byte{word, sheet, locked, old, []byte("PK\x03\x04 zip")} {
		said := templateUploadHint(data)
		if said == "" || !strings.ContainsAny(said, "가나다라마바사아자차카타파하업다니") {
			t.Errorf("a refusal reaches the reader in English: %q", said)
		}
	}
}
