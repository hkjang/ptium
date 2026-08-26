package pdftext

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"
)

// A PDF is an upload from whoever sends one, and this reader is the first thing
// that touches it. Two hundred kilobytes of well-chosen zlib expands to
// hundreds of megabytes, and one page is free to name the same stream sixty
// times — so a file this size can ask for gigabytes and for as long as it likes.
func TestAPageCannotAskForMoreThanItCanBeGiven(t *testing.T) {
	var packed bytes.Buffer
	writer := zlib.NewWriter(&packed)
	block := bytes.Repeat([]byte(" "), 1<<20)
	for index := 0; index < 100; index++ { // 100 MiB of spaces
		if _, err := writer.Write(block); err != nil {
			t.Fatal(err)
		}
	}
	writer.Close()

	var references string
	for index := 0; index < 60; index++ {
		references += "4 0 R "
	}
	var out bytes.Buffer
	out.WriteString("%PDF-1.7\n1 0 obj\n<</Type /Catalog /Pages 2 0 R>>\nendobj\n" +
		"2 0 obj\n<</Type /Pages /Kids [3 0 R] /Count 1>>\nendobj\n")
	fmt.Fprintf(&out, "3 0 obj\n<</Type /Page /Parent 2 0 R /Contents [%s]>>\nendobj\n", references)
	fmt.Fprintf(&out, "4 0 obj\n<</Filter /FlateDecode /Length %d>>\nstream\n", packed.Len())
	out.Write(packed.Bytes())
	out.WriteString("\nendstream\nendobj\ntrailer<</Root 1 0 R>>\n")

	if out.Len() > 1<<20 {
		t.Fatalf("the crafted file is %d bytes; the point is that it is small", out.Len())
	}
	done := make(chan struct{})
	var allocated uint64
	go func() {
		defer close(done)
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		if _, err := Read(out.Bytes()); err != nil {
			t.Errorf("Read() error = %v", err)
		}
		runtime.ReadMemStats(&after)
		allocated = after.TotalAlloc - before.TotalAlloc
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("a file smaller than a megabyte took half a minute to read")
	}
	// Both bounds matter, and each catches something the other does not: the
	// clock catches unpacking the same stream over and over, and the tape
	// measure catches one page taking everything it is handed.
	if limit := uint64(256 << 20); allocated > limit {
		t.Errorf("a %d byte file allocated %d MiB, want under %d MiB",
			out.Len(), allocated>>20, limit>>20)
	}
}

// A document too big to unpack in one go is not a document whose later pages
// are blank. What was not read has to be countable, or the import will describe
// pages nobody looked at — and send somebody off to re-export a file that reads
// perfectly well.
func TestADocumentTooBigToReadSaysSoRatherThanComingBackEmpty(t *testing.T) {
	var page strings.Builder
	for row := 0; row < 4000; row++ {
		fmt.Fprintf(&page, "BT /F1 12 Tf 1 0 0 1 72 %d Tm (line %d of a long report page) Tj ET\n", 700-row, row)
	}
	var packed bytes.Buffer
	writer := zlib.NewWriter(&packed)
	if _, err := writer.Write([]byte(page.String())); err != nil {
		t.Fatal(err)
	}
	writer.Close()

	const pages = 300
	var out bytes.Buffer
	out.WriteString("%PDF-1.7\n1 0 obj\n<</Type /Catalog /Pages 2 0 R>>\nendobj\n")
	kids := make([]string, 0, pages)
	for index := 0; index < pages; index++ {
		kids = append(kids, fmt.Sprintf("%d 0 R", 3+index*2))
	}
	fmt.Fprintf(&out, "2 0 obj\n<</Type /Pages /Kids [%s] /Count %d>>\nendobj\n", strings.Join(kids, " "), pages)
	font := 3 + pages*2
	for index := 0; index < pages; index++ {
		// Each page owns its own copy, the way a real long document does.
		fmt.Fprintf(&out, "%d 0 obj\n<</Type /Page /Parent 2 0 R /Resources <</Font <</F1 %d 0 R>>>> /Contents %d 0 R>>\nendobj\n",
			3+index*2, font, 4+index*2)
		fmt.Fprintf(&out, "%d 0 obj\n<</Filter /FlateDecode /Length %d>>\nstream\n", 4+index*2, packed.Len())
		out.Write(packed.Bytes())
		out.WriteString("\nendstream\nendobj\n")
	}
	fmt.Fprintf(&out, "%d 0 obj\n<</Type /Font /Subtype /Type1 /BaseFont /Helvetica>>\nendobj\ntrailer<</Root 1 0 R>>\n", font)

	read, err := Read(out.Bytes())
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if !read.Short {
		t.Fatalf("a document unpacking past the budget did not say so: %d of %d pages",
			len(read.Pages), read.Total)
	}
	if read.Total != pages {
		t.Errorf("the file has %d pages and the reading says %d", pages, read.Total)
	}
	if len(read.Pages) == 0 || len(read.Pages) >= pages {
		t.Errorf("%d of %d pages read; want the front of the document", len(read.Pages), read.Total)
	}
	// Every page handed back was read whole. A page cut off in the middle is
	// not what the page says.
	for _, held := range read.Pages {
		if len(held.Lines) == 0 {
			t.Errorf("page %d came back empty from a document that is all text", held.Number)
			break
		}
	}
}

// The same stream is not unpacked twice. A long file names one twelve-thousand
// entry map on every page, and reading it again for each was most of the time
// such a file took.
func TestAStreamIsUnpackedOnce(t *testing.T) {
	cmap := koreanCMap(map[uint32]rune{1: '보', 2: '고', 3: '서'}, 0)
	data := build(
		"<</Type /Catalog /Pages 2 0 R>>",
		"<</Type /Pages /Kids [3 0 R 8 0 R] /Count 2>>",
		"<</Type /Page /Parent 2 0 R /Resources <</Font <</F1 5 0 R>>>> /Contents 4 0 R>>",
		streamed("", "BT /F1 14 Tf 1 0 0 1 72 700 Tm <000100020003> Tj ET"),
		"<</Type /Font /Subtype /Type0 /BaseFont /Noto /Encoding /Identity-H /DescendantFonts [7 0 R] /ToUnicode 6 0 R>>",
		streamed("", cmap),
		"<</Type /Font /Subtype /CIDFontType0 /BaseFont /Noto>>",
		"<</Type /Page /Parent 2 0 R /Resources <</Font <</F1 5 0 R>>>> /Contents 9 0 R>>",
		streamed("", "BT /F1 14 Tf 1 0 0 1 72 700 Tm <000300020001> Tj ET"),
	)
	doc, err := open(data)
	if err != nil {
		t.Fatalf("open() error = %v", err)
	}
	for _, page := range doc.pages() {
		doc.fontsOf(page)
		doc.contentOf(page)
	}
	spent := doc.spent
	for _, page := range doc.pages() {
		doc.fontsOf(page)
		doc.contentOf(page)
	}
	if doc.spent != spent {
		t.Errorf("reading the same pages again unpacked another %d bytes", doc.spent-spent)
	}
	if len(doc.charted) != 1 {
		t.Errorf("the map was parsed %d times, want once", len(doc.charted))
	}
}

// A damaged file — truncated by a transfer, cut about by a repair tool, or
// simply not what it says it is — comes back as an error or as empty pages.
// Never as a panic, and never as a wait.
func TestADamagedFileIsStillAnswered(t *testing.T) {
	original := build(
		"<</Type /Catalog /Pages 2 0 R>>",
		"<</Type /Pages /Kids [3 0 R] /Count 1>>",
		"<</Type /Page /Parent 2 0 R /Resources <</Font <</F1 5 0 R>>>> /Contents 4 0 R>>",
		streamed("", "BT /F1 12 Tf 1 0 0 1 72 700 Tm (Quarterly review) Tj ET\nBT /F1 12 Tf 1 0 0 1 72 680 Tm (Second line) Tj ET"),
		"<</Type /Font /Subtype /Type1 /BaseFont /Helvetica>>",
	)
	seed := uint64(0x2545F4914F6CDD1D)
	next := func() uint64 {
		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17
		return seed
	}
	for attempt := 0; attempt < 300; attempt++ {
		data := append([]byte{}, original...)
		switch attempt % 3 {
		case 0:
			data = data[:int(next()%uint64(len(data)))]
		case 1:
			for flip := 0; flip < 8; flip++ {
				data[int(next()%uint64(len(data)))] ^= byte(next())
			}
		case 2:
			at := int(next() % uint64(len(data)))
			end := at + int(next()%64)
			if end > len(data) {
				end = len(data)
			}
			data = append(data[:at:at], data[end:]...)
		}
		func() {
			defer func() {
				if panicked := recover(); panicked != nil {
					t.Fatalf("attempt %d panicked on %d bytes: %v", attempt, len(data), panicked)
				}
			}()
			_, _ = Read(data)
		}()
	}
}
