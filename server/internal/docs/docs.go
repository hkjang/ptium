// Package docs reads the documents a company already has into deck source.
//
// Ptium's premise is that the material for a deck already exists: last
// quarter's report is a .docx, the numbers are an .xlsx, the notes are
// markdown. Asking someone to retype all of it into a brief is asking them to
// do the work twice, and what they retype loses the one thing that matters
// most — where each figure came from.
//
// So a document is read into the same deck source everything else compiles
// from, and every slide it produces cites the file and the place in it. What
// cannot be read is said out loud rather than guessed at.
//
// Everything here is the standard library. A deployment with no network and no
// system packages reads these files the same way one with everything does.
package docs

import (
	"fmt"
	"path"
	"strings"
)

// Document is a file, read as deck source.
type Document struct {
	// Title is what the file called itself, when it said.
	Title string
	// Source is deck source: the same language the editor edits.
	Source string
	// Warnings are for the person who uploaded the file, in their language.
	Warnings []string
}

// ErrUnsupported says the file is not one this package reads.
type ErrUnsupported struct{ Extension string }

func (e ErrUnsupported) Error() string {
	return fmt.Sprintf("no reader for %q", e.Extension)
}

// Extensions are what Read accepts, for the upload dialog to name.
var Extensions = []string{".csv", ".tsv", ".xlsx", ".docx", ".pdf", ".md", ".markdown", ".txt"}

// Reads reports whether a file name is one this package can read.
func Reads(filename string) bool {
	extension := strings.ToLower(path.Ext(strings.TrimSpace(filename)))
	for _, candidate := range Extensions {
		if extension == candidate {
			return true
		}
	}
	return false
}

// Read turns a file into deck source.
func Read(filename string, data []byte) (Document, error) {
	name := strings.TrimSpace(path.Base(filename))
	switch strings.ToLower(path.Ext(name)) {
	case ".csv":
		return readSeparated(name, data, ',')
	case ".tsv":
		return readSeparated(name, data, '\t')
	case ".xlsx":
		return readWorkbook(name, data)
	case ".docx":
		return readWordDocument(name, data)
	case ".pdf":
		return readPDF(name, data)
	case ".md", ".markdown", ".txt":
		return readMarkdown(name, data)
	}
	return Document{}, ErrUnsupported{Extension: path.Ext(name)}
}

// titleOf is what to call a deck made from a file: the file's own name,
// without the extension.
func titleOf(filename string) string {
	name := strings.TrimSpace(path.Base(filename))
	if extension := path.Ext(name); extension != "" {
		name = strings.TrimSuffix(name, extension)
	}
	if name == "" {
		return "가져온 문서"
	}
	return name
}

// A slide built from a file says which file, and where in it. The locator is
// what someone would type to find it again: a sheet and a range, a heading.
func citation(filename, locator string) string {
	if strings.TrimSpace(locator) == "" {
		return fmt.Sprintf("!source %s\n", escapeField(filename))
	}
	return fmt.Sprintf("!source %s | %s\n", escapeField(filename), escapeField(locator))
}

// escapeField protects a value from being read as another field.
func escapeField(value string) string {
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(value, "|", "/"), "\n", " "))
}

// escapeNote fits a note onto the one line a "!notes" directive occupies.
//
// A note is not a field: nothing after "!notes" is a separator, so the bar in
// "단가 | 수수료 | 거래세" is the document's own punctuation and turning it into a
// slash rewrites what somebody wrote. Sixteen lines of the files this was
// measured against are written exactly that way.
func escapeNote(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

// escapeLine protects text from being read as a directive.
func escapeLine(value string) string {
	trimmed := strings.TrimSpace(strings.ReplaceAll(value, "\n", " "))
	if trimmed == "" {
		return trimmed
	}
	switch trimmed[0] {
	case '#', '\\', '-', '*', '>', '!', '@':
		return "\\" + trimmed
	}
	return trimmed
}

// maximum sizes. A document is material for a deck, not the deck: a hundred
// rows of a spreadsheet on one slide is a screenshot of a spreadsheet.
const (
	maximumSlides  = 30
	maximumRows    = 8
	maximumColumns = 5
	// A sheet that runs longer than this is a spreadsheet, not a deck: what is
	// past it is said rather than drawn.
	maximumTableSlides = 4
	maximumPoints      = 5
)
