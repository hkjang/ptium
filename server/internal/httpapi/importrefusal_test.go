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
	// A zip that is simply not a presentation has no hint, and the import path
	// falls through to the document reader as it always did.
	if said := templateUploadHint([]byte("PK\x03\x04 an ordinary zip")); said != "" {
		t.Errorf("an ordinary package was given a wrapper's message: %q", said)
	}
}
