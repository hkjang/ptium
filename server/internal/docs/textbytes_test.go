package docs

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// Excel in Korean writes CP949 unless somebody opens the encoding menu, and
// "save as UTF-8" is a menu most people never open. Those bytes used to travel
// through the reader untouched and be refused by the database, so uploading an
// ordinary spreadsheet ended in a server error naming nothing anyone could act
// on.
func TestAFileIsReadInWhateverItWasSavedAs(t *testing.T) {
	const said = "항목,작년,올해\n매출,103억,128억\n"
	for _, one := range []struct {
		what string
		data []byte
	}{
		{"UTF-8", []byte(said)},
		{"UTF-8 with a byte-order mark", append([]byte{0xEF, 0xBB, 0xBF}, said...)},
		{"CP949", toCP949(t, said)},
		{"UTF-16 little-endian", toUTF16(said, false)},
		{"UTF-16 big-endian", toUTF16(said, true)},
	} {
		text, ok := asText(one.data)
		if !ok {
			t.Errorf("%s: the bytes were not read as text at all", one.what)
			continue
		}
		if text != said {
			t.Errorf("%s: read as %q, want %q", one.what, text, said)
		}
	}
}

func TestTheWholeDocumentSurvivesItsEncoding(t *testing.T) {
	document, err := Read("매출.csv", toCP949(t, "항목,작년,올해\n매출,103억,128억\n"))
	if err != nil {
		t.Fatalf("reading a CP949 spreadsheet: %v", err)
	}
	for _, want := range []string{"항목", "작년", "103억", "128억"} {
		if !strings.Contains(document.Source, want) {
			t.Errorf("the deck does not say %q:\n%s", want, document.Source)
		}
	}
}

// toCP949 writes the text the way Excel would, without depending on a decoder
// to check a decoder: the bytes are the ones a Korean Windows machine writes.
func toCP949(t *testing.T, text string) []byte {
	t.Helper()
	// Round-tripping through the same table the reader uses would prove nothing
	// on its own, so the result is checked to be bytes UTF-8 cannot read.
	encoded, err := cp949(text)
	if err != nil {
		t.Fatalf("writing CP949: %v", err)
	}
	if utf8.Valid(encoded) {
		t.Fatalf("these bytes are readable as UTF-8, so they test nothing: %q", encoded)
	}
	return encoded
}

func toUTF16(text string, bigEndian bool) []byte {
	units := []rune(text)
	out := []byte{0xFF, 0xFE}
	if bigEndian {
		out = []byte{0xFE, 0xFF}
	}
	for _, unit := range units {
		if bigEndian {
			out = append(out, byte(unit>>8), byte(unit))
			continue
		}
		out = append(out, byte(unit), byte(unit>>8))
	}
	return out
}
