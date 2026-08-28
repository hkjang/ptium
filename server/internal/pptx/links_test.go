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
	// The address belongs in the link, never in the words: drawn text is what
	// sits between the tags.
	if strings.Contains(svg, ">https://docs.example.com/a") || strings.Contains(svg, "](") {
		t.Errorf("the preview draws the markup: %s", svg)
	}
	if !strings.Contains(svg, `<a href="https://docs.example.com/a" target="_blank"`) {
		t.Errorf("the drawing is not a link where the page can follow it: %s", svg)
	}
	if !strings.Contains(svg, "안내 문서") || !strings.Contains(svg, "자세한 내용은") {
		t.Errorf("the preview lost the words: %s", svg)
	}
	if !strings.Contains(svg, `text-decoration="underline"`) {
		t.Errorf("the link is not drawn as one: %s", svg)
	}
}

// The words are drawn, the markup is not — unless the target is one the deck
// will not follow, and then the markup is what a room reads off the wall.
func TestALinkTheDeckWillNotFollowIsReported(t *testing.T) {
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
	deck := Deck{Slides: []Slide{{LayoutID: layout.ID, Fields: map[string][]Paragraph{
		SlotTitle: {{Text: "안내"}},
		SlotBody: {
			{Text: "[문서](www.example.com)를 보십시오"},
			{Text: "[제대로 된 것](https://example.com)"},
			{Text: "각주 [1] 참조"},
		},
	}}}}
	var reported []Finding
	for _, finding := range InspectDeck(manifest, deck) {
		if finding.Kind == FindingLink {
			reported = append(reported, finding)
		}
	}
	if len(reported) != 1 {
		t.Fatalf("expected one refused link, got %d: %#v", len(reported), reported)
	}
	if !strings.Contains(reported[0].Detail, "www.example.com") || !reported[0].Advisory {
		t.Errorf("the finding does not say what was refused: %#v", reported[0])
	}
	if reported[0].Slide != 1 || reported[0].Slot != SlotBody {
		t.Errorf("the finding does not say where it is: %#v", reported[0])
	}
}

func TestAWordCanBeMarkedInsideALine(t *testing.T) {
	runs := SplitRuns("이번 분기 **매출이 12% 늘었습니다**. *가정은 인건비 동결입니다*")
	if len(runs) != 4 {
		t.Fatalf("expected four runs, got %d: %#v", len(runs), runs)
	}
	if runs[1].Text != "매출이 12% 늘었습니다" || !runs[1].Bold || runs[1].Italic {
		t.Errorf("the bold run is wrong: %#v", runs[1])
	}
	if runs[3].Text != "가정은 인건비 동결입니다" || !runs[3].Italic || runs[3].Bold {
		t.Errorf("the italic run is wrong: %#v", runs[3])
	}
	if plain := PlainText("이번 분기 **매출이 12% 늘었습니다**"); plain != "이번 분기 매출이 12% 늘었습니다" {
		t.Errorf("what the slide draws is %q", plain)
	}
}

// A star is a star. Arithmetic, a footnote marker and a bullet somebody typed
// have to come back as themselves.
func TestAStarWithNoPartnerIsAStar(t *testing.T) {
	for _, text := range []string{
		"3 * 4 = 12",
		"매출 * 원가",
		"주석 *",
		"*",
		"**",
		`\*강조가 아닙니다\*`,
		"* 항목",
	} {
		runs := SplitRuns(text)
		for _, run := range runs {
			if run.Bold || run.Italic {
				t.Errorf("%q was read as emphasis: %#v", text, runs)
			}
		}
		want := strings.ReplaceAll(text, `\*`, "*")
		if PlainText(text) != want {
			t.Errorf("%q draws as %q", text, PlainText(text))
		}
	}
}

// A link is a place in the line, and a mark is something true of the words
// there: the two have to be able to hold at once.
func TestAMarkedLink(t *testing.T) {
	runs := SplitRuns("**[안내 문서](https://docs.example.com/a)**")
	if len(runs) != 1 {
		t.Fatalf("expected one run, got %#v", runs)
	}
	if !runs[0].Bold || runs[0].Href != "https://docs.example.com/a" || runs[0].Text != "안내 문서" {
		t.Errorf("the run lost half of what it is: %#v", runs[0])
	}
	inner := SplitRuns("[**안내** 문서](https://docs.example.com/a)")
	if len(inner) != 2 || !inner[0].Bold || inner[1].Bold {
		t.Fatalf("a mark inside a label did not read: %#v", inner)
	}
	if inner[0].Href != inner[1].Href || inner[0].Href == "" {
		t.Errorf("both halves of the label are the same link: %#v", inner)
	}
}

// The region says one thing about all of its runs and the author says another
// about one of them. Both are stated once.
func TestAMarkOnTopOfWhatTheRegionAlreadySays(t *testing.T) {
	properties := `<a:rPr lang="ko-KR" dirty="0" b="0" i="1"/>`
	marked := runsXML("보통 **굵게**", properties, nil)
	if strings.Count(marked, `b="0"`) != 1 || strings.Count(marked, `b="1"`) != 1 {
		t.Errorf("the region's own weight was not replaced for the marked run: %s", marked)
	}
	for _, run := range strings.Split(marked, "<a:rPr")[1:] {
		if end := strings.Index(run, ">"); end >= 0 {
			run = run[:end]
		}
		if strings.Count(run, ` b="`) != 1 || strings.Count(run, ` i="`) != 1 {
			t.Errorf("a run states an attribute twice: <a:rPr%s>", run)
		}
	}
	// Nothing marked, nothing changed: the common line is written as it was.
	if plain := runsXML("보통", properties, nil); plain != `<a:r>`+properties+`<a:t>보통</a:t></a:r>` {
		t.Errorf("an unmarked line was rewritten: %s", plain)
	}
}

// A jump names the slide it goes to. The page around the drawing is what takes
// somebody there, so what the drawing has to say is which slide.
func TestThePreviewNamesTheSlideAJumpGoesTo(t *testing.T) {
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
		SlotBody:  {{Text: "근거는 [부록](#7)에 있습니다"}},
	}}
	svg := PreviewSVG(manifest, layout, slide, PreviewOptions{Width: 900})
	if !strings.Contains(svg, `<a href="#slide-7"`) {
		t.Errorf("the jump does not name its slide: %s", svg)
	}
	if strings.Contains(svg, ">#7<") || strings.Contains(svg, "](") {
		t.Errorf("the preview draws the markup: %s", svg)
	}
}

// "1200*800*750" is how a size is written in a Korean deck — 가로*세로*높이 — and
// the marks were read as emphasis, so the slide drew 1200800750. Markup printed
// on the wall is caught by anyone who looks; a number quietly turned into a
// different number is caught by nobody.
func TestAStarPressedAgainstAWordIsNotAMark(t *testing.T) {
	for _, typed := range []string{
		"본체는 1200*800*750 입니다",
		"규격(가로*세로*높이)",
		"12*34*56",
		"수량 3*단가 5*계수 2",
	} {
		if drawn := PlainText(typed); drawn != typed {
			t.Errorf("the author typed %q and the slide draws %q", typed, drawn)
		}
		for _, run := range SplitRuns(typed) {
			if run.Bold || run.Italic {
				t.Errorf("%q was read as emphasis on %q", typed, run.Text)
			}
		}
	}
}

// And the emphasis this product is actually written with still reads as
// emphasis — including the Korean particle that hangs straight off the closing
// mark, which is why only the opening side is tested.
func TestTheEmphasisPeopleWriteStillReads(t *testing.T) {
	cases := map[string]string{
		"이 항목은 **중요**합니다":  "중요",
		"**중요**한 사항":       "중요",
		"(**필수**) 항목":      "필수",
		"비고: *검토 필요*":      "검토 필요",
		"**굵게** 그리고 *기울임*": "굵게",
	}
	for typed, marked := range cases {
		found := false
		for _, run := range SplitRuns(typed) {
			if (run.Bold || run.Italic) && run.Text == marked {
				found = true
			}
		}
		if !found {
			t.Errorf("%q no longer marks %q", typed, marked)
		}
		if strings.Contains(PlainText(typed), "*") {
			t.Errorf("%q draws its marks: %q", typed, PlainText(typed))
		}
	}
}

// The same mistake in the same deck got two answers: written in a point it was
// reported, and written in a cell of a table it drew [계약서](www.example.com)
// on the wall in silence. A component's words are on the slide too.
func TestARefusedLinkInAComponentIsReportedToo(t *testing.T) {
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
	deck := Deck{Slides: []Slide{{LayoutID: layout.ID,
		Fields: map[string][]Paragraph{SlotTitle: {{Text: "근거"}}},
		Blocks: map[string]Block{SlotBody: {Kind: BlockTable,
			Columns: []string{"항목", "근거"},
			Rows: [][]string{
				{"인프라", "[계약서](www.example.com)"},
				{"인건비", "[승인서](https://example.com/ok)"},
				{"기타", "각주 [1] 참조"},
			}}},
	}}}
	var reported []Finding
	for _, finding := range InspectDeck(manifest, deck) {
		if finding.Kind == FindingLink {
			reported = append(reported, finding)
		}
	}
	if len(reported) != 1 {
		t.Fatalf("expected one refused link from the table, got %d: %#v", len(reported), reported)
	}
	if !strings.Contains(reported[0].Detail, "www.example.com") || !reported[0].Advisory {
		t.Errorf("the finding does not say what was refused: %#v", reported[0])
	}
	if reported[0].Slide != 1 || reported[0].Slot != SlotBody {
		t.Errorf("the finding does not say where it is: %#v", reported[0])
	}
}

// Every shape a component takes carries words somebody typed, so the walk is
// over the words rather than over the shapes it happens to know about.
func TestAComponentsWordsAreAllWalked(t *testing.T) {
	block := Block{Kind: BlockKPI, Heading: "제목", Caption: "설명", Attribute: "출처",
		Text: "한 문장", Labels: []string{"라벨"}, Columns: []string{"열"},
		Rows:   [][]string{{"칸"}},
		Items:  []Item{{Label: "이름", Value: "값", Delta: "+3%", Detail: "자세히", Bullets: []string{"점"}}},
		Series: []Series{{Name: "계열"}}}
	found := map[string]bool{}
	for _, word := range blockWords(block) {
		found[word] = true
	}
	for _, want := range []string{"제목", "설명", "출처", "한 문장", "라벨", "열", "칸",
		"이름", "값", "+3%", "자세히", "점", "계열"} {
		if !found[want] {
			t.Errorf("a reader sees %q on the slide and the walk does not", want)
		}
	}
}

// A deck of three slides linking to its ninth wrote a relationship at
// ppt/slides/slide9.xml — a part that is not in the package it wrote. Nothing
// can follow that, and a reader is entitled to call the file broken. Slides get
// cut after the links to them are written, and nothing said so.
func TestAJumpToASlideTheDeckDoesNotHaveIsNotWritten(t *testing.T) {
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
	slide := func(title, body string) Slide {
		return Slide{LayoutID: layout.ID, Fields: map[string][]Paragraph{
			SlotTitle: {{Text: title}}, SlotBody: {{Text: body}}}}
	}
	deck := Deck{Language: "ko", Slides: []Slide{
		slide("첫 장", "뒤쪽 [9장 참고](#9) 과 있는 장 [2장](#2)"),
		slide("둘째 장", "내용"),
		slide("셋째 장", "내용"),
	}}
	built, err := RenderBytes(data, deck)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := Open(built)
	if err != nil {
		t.Fatal(err)
	}
	rels, ok := pkg.Text("ppt/slides/_rels/slide1.xml.rels")
	if !ok {
		t.Fatal("the first slide has no relationships")
	}
	if strings.Contains(rels, "slide9.xml") {
		t.Error("the package names a slide part it does not contain")
	}
	if !strings.Contains(rels, "slide2.xml") {
		t.Error("a jump the deck can follow was dropped with the one it cannot")
	}
	// The words are the author's either way.
	first, _ := pkg.Text("ppt/slides/slide1.xml")
	for _, word := range []string{"9장 참고", "2장"} {
		if !strings.Contains(first, word) {
			t.Errorf("the words %q are not on the slide", word)
		}
	}

	// And the author is told, rather than finding out from a reader.
	said := ""
	for _, finding := range InspectDeck(manifest, deck) {
		if finding.Kind == FindingLink && finding.Slide == 1 {
			said = finding.Detail
		}
	}
	if !strings.Contains(said, "9장 참고") || !strings.Contains(said, "3") {
		t.Errorf("the dangling jump was not reported clearly: %q", said)
	}
}
