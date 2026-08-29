package pptx

import (
	"crypto/sha256"
	"encoding/hex"
	"html"
	"regexp"
	"sort"
	"strings"
)

// A deck Ptium exports carries the text it was written from.
//
// Everything on a slide is drawn — a process is circles and lines, a KPI row is
// boxes and numbers — and reading that back turns a component into a list of
// stray labels: "1 · 준비 · 범위 확정 · 2 · 이행". The words survive and the
// meaning does not. So the export keeps the deck's own source in the file, in a
// part PowerPoint ignores, and importing a Ptium deck restores what was written
// rather than guessing at it from the drawing.
//
// The part is small (a few kilobytes of text), invisible in PowerPoint, and
// optional: a file that has lost it — because someone re-saved it elsewhere, or
// because it was never a Ptium deck — is read from its shapes as before.
const (
	sourcePart         = "ppt/ptiumSource.xml"
	sourceRelationship = "https://ptium.dev/2026/relationships/deckSource"
	sourceNamespace    = "https://ptium.dev/2026/source"
)

// writeDeckSource stores the deck's source in the package and relates it to the
// presentation, so the part travels with the file rather than being orphaned.
func writeDeckSource(pkg *Package, source string) string {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		pkg.Delete(sourcePart)
		return ""
	}
	pkg.SetText(sourcePart, xmlDeclaration+
		`<deckSource xmlns="`+sourceNamespace+`" version="1" slides="`+deckTextDigest(pkg)+`">`+
		escapeSourceText(trimmed)+`</deckSource>`)
	return sourcePart
}

// deckTextDigest is a fingerprint of every word the deck carries, on its slides
// and in its speaker notes alike.
//
// Someone who opens the exported file in PowerPoint, fixes a number and sends
// it back has made the embedded source out of date. Restoring from it would
// throw their edit away without saying so — the worst thing this feature could
// do — so the file is checked against the fingerprint first, and one that no
// longer matches is read from its shapes like anyone else's.
//
// The notes count. Fingerprinting only the slides let an edit made in the notes
// pane through: the words on the slides still matched, the source was restored
// whole, and what the speaker had written to say was gone with nothing said
// about it. Notes are written into the package before this runs, so they are
// there to be read.
func deckTextDigest(pkg *Package) string {
	names := append(pkg.NamesUnder("ppt/slides/"), pkg.NamesUnder("ppt/notesSlides/")...)
	sort.Strings(names)
	digest := sha256.New()
	for _, name := range names {
		if strings.Contains(name, "/_rels/") {
			continue
		}
		content, ok := pkg.Text(name)
		if !ok {
			continue
		}
		digest.Write([]byte(name))
		for _, match := range textRun.FindAllStringSubmatch(content, -1) {
			digest.Write([]byte(match[1]))
			digest.Write([]byte{0})
		}
	}
	return hex.EncodeToString(digest.Sum(nil))[:32]
}

// textRun is one run of text on a slide, which is where every word a reader
// sees lives.
var textRun = regexp.MustCompile(`<a:t>([^<]*)</a:t>`)

// DeckSource is the source a Ptium export carried, if this file is one.
func DeckSource(pkg *Package) (string, bool) {
	content, ok := pkg.Text(sourcePart)
	if !ok {
		return "", false
	}
	// The first ">" in the part belongs to the XML declaration, not to the
	// element: looking for it there put "<deckSource …>" at the top of every
	// imported deck.
	start := strings.Index(content, "<deckSource")
	if start < 0 {
		return "", false
	}
	opening := strings.Index(content[start:], ">")
	closing := strings.LastIndex(content, "</deckSource>")
	if opening < 0 {
		return "", false
	}
	opening += start
	if closing <= opening {
		return "", false
	}
	// The part is written by this package and read by it: the only markup inside
	// is the escaping applied above.
	// A file edited somewhere else no longer says what this source says, and the
	// author's edit is worth more than the components the source would restore.
	if recorded := digestAttribute(content[start:opening]); recorded != "" && recorded != deckTextDigest(pkg) {
		return "", false
	}
	source := strings.TrimSpace(html.UnescapeString(content[opening+1 : closing]))
	if source == "" {
		return "", false
	}
	return source, true
}

// digestAttribute reads the fingerprint out of the opening tag.
func digestAttribute(tag string) string {
	match := slidesAttribute.FindStringSubmatch(tag)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

var slidesAttribute = regexp.MustCompile(`slides="([0-9a-f]*)"`)

// escapeSourceText escapes the characters XML reserves while keeping the line
// breaks. The renderer's own escaper turns a newline into a space, which is
// right for a line of text on a slide and wrong for a document whose lines are
// its structure.
func escapeSourceText(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for _, symbol := range value {
		switch {
		case symbol == '\n' || symbol == '\t':
			builder.WriteRune(symbol)
		case symbol == '\r':
			continue
		case symbol < 0x20:
			continue
		default:
			builder.WriteString(html.EscapeString(string(symbol)))
		}
	}
	return builder.String()
}
