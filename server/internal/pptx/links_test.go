package pptx

import (
	"strings"
	"testing"
)

func TestAParagraphSplitsIntoTheRunsItDraws(t *testing.T) {
	runs := SplitLinks("자세한 내용은 [안내 문서](https://docs.example.com/a?b=1#c)를 보십시오.")
	if len(runs) != 3 {
		t.Fatalf("expected three runs, got %d: %#v", len(runs), runs)
	}
	if runs[0].Text != "자세한 내용은 " || runs[0].Href != "" {
		t.Errorf("the words before the link changed: %#v", runs[0])
	}
	if runs[1].Text != "안내 문서" || runs[1].Href != "https://docs.example.com/a?b=1#c" {
		t.Errorf("the link is wrong: %#v", runs[1])
	}
	if runs[2].Text != "를 보십시오." {
		t.Errorf("the words after the link changed: %#v", runs[2])
	}
	if plain := PlainText("자세한 내용은 [안내 문서](https://docs.example.com/a?b=1#c)를 보십시오."); plain != "자세한 내용은 안내 문서를 보십시오." {
		t.Errorf("what the slide draws is %q", plain)
	}
}

// Nearly every paragraph has no link in it, and has to come back untouched.
func TestTextWithNoLinkIsOneRun(t *testing.T) {
	for _, text := range []string{
		"3분기 매출은 12% 늘었습니다",
		"각주 [1] 참조",
		"조건 (가) 또는 (나)",
		"목록 [가](나) 비교",             // a target nothing can point at
		"[](https://example.com)",  // nothing to click
		"[말](javascript:alert(1))", // not a scheme the deck will follow
		"[말](ftp://example.com)",
		"[말](https://)",
		"[말](/사내/문서)",
		"[말] (https://example.com)",
	} {
		runs := SplitLinks(text)
		if len(runs) != 1 || runs[0].Href != "" || runs[0].Text != text {
			t.Errorf("%q became %#v", text, runs)
		}
		if PlainText(text) != text {
			t.Errorf("%q measures as %q", text, PlainText(text))
		}
		if HasLink(text) {
			t.Errorf("%q is not a link", text)
		}
	}
}

func TestABracketCanBeWrittenLiterally(t *testing.T) {
	runs := SplitLinks(`\[말](https://example.com)`)
	if len(runs) != 1 || runs[0].Href != "" || runs[0].Text != "[말](https://example.com)" {
		t.Fatalf("an escaped bracket became %#v", runs)
	}
	if PlainText(`\[말](https://example.com)`) != "[말](https://example.com)" {
		t.Errorf("the escape is drawn: %q", PlainText(`\[말](https://example.com)`))
	}
}

func TestALinkToAnotherSlide(t *testing.T) {
	runs := SplitLinks("근거는 [부록](#7)에 있습니다")
	if len(runs) != 3 || runs[1].Href != "#7" {
		t.Fatalf("a slide jump did not read: %#v", runs)
	}
	if number, ok := SlideJump("#7"); !ok || number != 7 {
		t.Errorf("SlideJump(#7) = %d, %v", number, ok)
	}
	for _, href := range []string{"#0", "#", "#-1", "#3장", "#99999", "https://example.com"} {
		if _, ok := SlideJump(href); ok {
			t.Errorf("%q is not a slide jump", href)
		}
	}
}

func TestTwoLinksInOneLine(t *testing.T) {
	runs := SplitLinks("[가](https://a.example)와 [나](mailto:b@example.com)")
	if len(runs) != 3 {
		t.Fatalf("expected three runs, got %#v", runs)
	}
	if runs[0].Href != "https://a.example" || runs[2].Href != "mailto:b@example.com" {
		t.Errorf("the two links did not both read: %#v", runs)
	}
	if runs[1].Text != "와 " || runs[1].Href != "" {
		t.Errorf("the words between them changed: %#v", runs[1])
	}
}

// An unfinished link is words, not a run that swallows the rest of the line.
func TestAnUnfinishedLinkIsLeftAlone(t *testing.T) {
	for _, text := range []string{"[말](https://example.com", "[말", "말](https://example.com)", "[[말]](https://a.example)"} {
		if HasLink(text) {
			t.Errorf("%q was read as a link: %#v", text, SplitLinks(text))
		}
		if PlainText(text) != text {
			t.Errorf("%q measures as %q", text, PlainText(text))
		}
	}
}

// The markup is longer than the words: a line that fits its region has to
// measure as fitting, or the workspace repairs a slide that draws well.
func TestALineIsMeasuredByItsWordsNotItsMarkup(t *testing.T) {
	data, err := BuiltinTemplate("")
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := AnalyzeBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	layout, ok := manifest.LayoutForRole(RoleContent)
	if !ok {
		t.Fatal("the builtin template has no content layout")
	}
	// A real report link, with the tracking a real one carries.
	long := "[분기 보고서](https://reports.example.com/2026/q3/summary?from=deck&utm_source=ptium&utm_medium=slide&utm_campaign=quarterly-review&section=figures&highlight=revenue&locale=ko-KR#figures)"
	body := make([]Paragraph, 4)
	for index := range body {
		body[index] = Paragraph{Text: long}
	}
	slide := Slide{LayoutID: layout.ID, Fields: map[string][]Paragraph{
		SlotTitle: {{Text: "근거"}},
		SlotBody:  body,
	}, Frames: map[string]Frame{SlotBody: {X: 800000, Y: 1500000, Width: 3000000, Height: 2000000}}}
	design := NewDesign(manifest)
	for _, finding := range InspectSlide(manifest, layout, slide, design) {
		if finding.Kind == FindingOverflow || finding.Kind == FindingOutside {
			t.Errorf("four short lines were measured as %s: %s", finding.Kind, finding.Detail)
		}
	}
	// And the slide the caller handed in is untouched.
	if slide.Fields[SlotBody][0].Text != long {
		t.Errorf("measuring rewrote the deck: %q", slide.Fields[SlotBody][0].Text)
	}
}

// The preview is what the rail, the share link and presenting all draw, so it
// is where a link has to look like one — and where the markup must never show.
func TestThePreviewDrawsTheWordsAsALink(t *testing.T) {
	data, err := BuiltinTemplate("")
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := AnalyzeBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	layout, ok := manifest.LayoutForRole(RoleContent)
	if !ok {
		t.Fatal("the builtin template has no content layout")
	}
	slide := Slide{LayoutID: layout.ID, Fields: map[string][]Paragraph{
		SlotTitle: {{Text: "근거"}},
		SlotBody:  {{Text: "자세한 내용은 [안내 문서](https://docs.example.com/a)를 보십시오"}},
	}}
	svg := PreviewSVG(manifest, layout, slide, PreviewOptions{Width: 900})
	if strings.Contains(svg, "https://docs.example.com/a") || strings.Contains(svg, "](") {
		t.Errorf("the preview draws the markup: %s", svg)
	}
	if !strings.Contains(svg, "안내 문서") || !strings.Contains(svg, "자세한 내용은") {
		t.Errorf("the preview lost the words: %s", svg)
	}
	if !strings.Contains(svg, `text-decoration="underline"`) {
		t.Errorf("the link is not drawn as one: %s", svg)
	}
}
