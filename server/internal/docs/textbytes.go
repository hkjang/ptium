package docs

import (
	"bytes"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/encoding/korean"
)

// A text file does not say what it is written in, so the bytes have to.
//
// Excel in Korean saves a spreadsheet as CP949 unless somebody chooses
// otherwise, and "save as UTF-8" is a menu most people never open. Those bytes
// were carried through the reader untouched and refused by the database —
// invalid byte sequence for encoding "UTF8" — so the upload ended in a server
// error that named nothing the person could act on. A file saved as UTF-16, the
// other thing that dialogue offers, ended the same way.
var (
	utf8BOM    = []byte{0xEF, 0xBB, 0xBF}
	utf16LEBOM = []byte{0xFF, 0xFE}
	utf16BEBOM = []byte{0xFE, 0xFF}
)

// asText reads the bytes as the text they are, and says so when it cannot.
//
// The order is what the bytes themselves can settle: a byte-order mark is a
// statement, valid UTF-8 is all but conclusive at any length, and what is left
// is tried as CP949 — the encoding the files that get this far are written in.
func asText(data []byte) (string, bool) {
	switch {
	case bytes.HasPrefix(data, utf8BOM):
		data = data[len(utf8BOM):]
	case bytes.HasPrefix(data, utf16LEBOM):
		return fromUTF16(data[len(utf16LEBOM):], false), true
	case bytes.HasPrefix(data, utf16BEBOM):
		return fromUTF16(data[len(utf16BEBOM):], true), true
	}
	if utf8.Valid(data) {
		return string(data), true
	}
	// CP949 is a superset of EUC-KR and this decoder reads both.
	if decoded, err := korean.EUCKR.NewDecoder().Bytes(data); err == nil && utf8.Valid(decoded) {
		return string(decoded), true
	}
	return "", false
}

// fromUTF16 reads the code units, in whichever order the mark said.
func fromUTF16(data []byte, bigEndian bool) string {
	units := make([]uint16, 0, len(data)/2)
	for at := 0; at+1 < len(data); at += 2 {
		if bigEndian {
			units = append(units, uint16(data[at])<<8|uint16(data[at+1]))
			continue
		}
		units = append(units, uint16(data[at+1])<<8|uint16(data[at]))
	}
	return string(utf16.Decode(units))
}

// cp949 writes text the way a Korean Windows machine does. It is here for the
// tests: nothing in the product writes CP949, it only reads it.
func cp949(text string) ([]byte, error) {
	return korean.EUCKR.NewEncoder().Bytes([]byte(text))
}
