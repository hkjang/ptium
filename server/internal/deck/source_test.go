package deck

import (
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

const sample = `# 클라우드 전환 로드맵
@cover
> 2026년 하반기 · 임원 보고
!notes 결론을 먼저 말합니다.
  이어서 근거를 두 가지로 좁힙니다.

// this line is a comment
# 이행 순서
@content
> 순서대로 나눠 봅니다.
::steps 3단계
- 준비 | 범위와 예산 확정
- 이행 | 워크로드 이관
- 안정화 | 운영 이관
::
- 각 단계는 병행할 수 없습니다
  - 선행 조건이 있기 때문입니다

# 기대 효과
::kpi 핵심 지표
- 전환 대상 | 42개
- 예상 절감 | 18%
::
!notes 지표는 세 개까지.
`

func TestParseSourceReadsEveryConstruct(t *testing.T) {
	parsed := ParseSource(sample)
	if len(parsed.Warnings) != 0 {
		t.Fatalf("warnings = %v", parsed.Warnings)
	}
	if len(parsed.Slides) != 3 {
		t.Fatalf("slides = %d", len(parsed.Slides))
	}
	cover := parsed.Slides[0]
	if cover.Title != "클라우드 전환 로드맵" || cover.Role != pptx.RoleTitle {
		t.Fatalf("cover = %+v", cover)
	}
	// Notes continue across the following lines until a blank line.
	if !strings.Contains(cover.Notes, "근거를 두 가지로") {
		t.Fatalf("notes did not continue: %q", cover.Notes)
	}

	steps := parsed.Slides[1]
	if len(steps.Blocks) != 1 || steps.Blocks[0].Kind != pptx.BlockSteps || steps.Blocks[0].Caption != "3단계" {
		t.Fatalf("block = %+v", steps.Blocks)
	}
	if len(steps.Blocks[0].Items) != 3 || steps.Blocks[0].Items[0].Value != "범위와 예산 확정" {
		t.Fatalf("items = %+v", steps.Blocks[0].Items)
	}
	// Bullets after a closed block belong to the slide, and nesting is read.
	if len(steps.Bullets) != 2 || steps.Bullets[1].Level != 1 {
		t.Fatalf("bullets = %+v", steps.Bullets)
	}

	kpi := parsed.Slides[2]
	if len(kpi.Blocks) != 1 || kpi.Blocks[0].Items[0].Number == nil || *kpi.Blocks[0].Items[0].Number != 42 {
		t.Fatalf("a numeric value should be read as a number: %+v", kpi.Blocks)
	}
}

func TestParseSourceIsForgiving(t *testing.T) {
	parsed := ParseSource("@nonsense\n# 제목\n@notaslidekind\n::notacomponent\n- 그냥 문장\n서술형 리드\n- 항목\n")
	if len(parsed.Slides) != 1 {
		t.Fatalf("slides = %+v", parsed.Slides)
	}
	if len(parsed.Warnings) < 2 {
		t.Fatalf("unknown directives should warn: %v", parsed.Warnings)
	}
	slide := parsed.Slides[0]
	// A bare line becomes the lead when there is none, a bullet afterwards.
	if slide.Lead != "" && slide.Lead != "서술형 리드" {
		t.Fatalf("lead = %q", slide.Lead)
	}
	if len(slide.Bullets) < 2 {
		t.Fatalf("bullets = %+v", slide.Bullets)
	}
}

func TestParseNumberReadsWrittenValues(t *testing.T) {
	cases := map[string]float64{
		"42개": 42, "18%": 18, "1,200억": 1200, "-3.5pt": -3.5, "12개월": 12, "0.5배": 0.5,
	}
	for value, want := range cases {
		got, ok := parseNumber(value)
		if !ok || got != want {
			t.Fatalf("parseNumber(%q) = %v, %v; want %v", value, got, ok, want)
		}
	}
	for _, value := range []string{"", "높음", "미정"} {
		if _, ok := parseNumber(value); ok {
			t.Fatalf("parseNumber(%q) should find no number", value)
		}
	}
}

func testManifest() pptx.Manifest {
	body := func(slot string, x int) pptx.Placeholder {
		return pptx.Placeholder{Slot: slot, Kind: "text", Type: "body", X: x, Y: 2000000,
			Width: 5000000, Height: 3000000, FontSize: 1800, MaxChars: 220, MaxLines: 9, LineEm: 26}
	}
	title := pptx.Placeholder{Slot: pptx.SlotTitle, Kind: "text", Type: "title", X: 800000, Y: 600000,
		Width: 10000000, Height: 1200000, FontSize: 3600, MaxChars: 60, MaxLines: 2, LineEm: 30}
	subtitle := pptx.Placeholder{Slot: pptx.SlotSubtitle, Kind: "text", Type: "subTitle", X: 800000, Y: 1900000,
		Width: 10000000, Height: 800000, FontSize: 2000, MaxChars: 90, MaxLines: 2, LineEm: 45}
	return pptx.Manifest{
		Version: pptx.ManifestVersion, SlideWidth: 12192000, SlideHeight: 6858000,
		Theme: pptx.Theme{Colors: map[string]string{"lt1": "FFFFFF", "dk1": "111111", "accent1": "1E6FFF"}},
		Layouts: []pptx.Layout{
			{ID: "cover", Name: "Cover", Role: pptx.RoleTitle, Placeholders: []pptx.Placeholder{title, subtitle}},
			{ID: "content", Name: "Content", Role: pptx.RoleContent, Placeholders: []pptx.Placeholder{title, subtitle, body(pptx.SlotBody, 800000)}},
			{ID: "two", Name: "Two content", Role: pptx.RoleTwoContent,
				Placeholders: []pptx.Placeholder{title, body(pptx.SlotBody, 700000), body("body2", 6300000)}},
			{ID: "closing", Name: "Closing", Role: pptx.RoleClosing, Placeholders: []pptx.Placeholder{title, subtitle, body(pptx.SlotBody, 800000)}},
		},
		TitleLayout: "cover", DefaultLayout: "content", ClosingLayout: "closing",
	}
}

func TestCompileBindsSourceToTheTemplate(t *testing.T) {
	manifest := testManifest()
	result := Compile(ParseSource(sample), manifest, CompileOptions{Language: "ko"})
	if len(result.Slides) != 3 {
		t.Fatalf("slides = %d, warnings %v", len(result.Slides), result.Warnings)
	}
	if result.Slides[0].LayoutID != "cover" {
		t.Fatalf("the cover should use the title layout: %q", result.Slides[0].LayoutID)
	}
	content := Decode(result.Slides[1].Content)
	if len(content.Blocks) != 1 {
		t.Fatalf("the steps component was not bound: %+v", content)
	}
	if _, ok := content.Blocks[pptx.SlotBody]; !ok {
		t.Fatalf("a component should claim the body slot: %+v", content.Blocks)
	}
	// The bullets that followed the block need a slot of their own; this layout
	// has one body region, so they are kept rather than dropped.
	if len(content.Fields[pptx.SlotBody]) > 0 {
		t.Fatal("prose and a component must not share one slot")
	}
	if strings.TrimSpace(content.Body) == "" && len(result.Warnings) == 0 {
		t.Fatalf("text with nowhere to go should be reported: %+v", result)
	}
	if result.Outline[0].Layout != "Cover" {
		t.Fatalf("outline = %+v", result.Outline)
	}
}

func TestCompileSpreadsBulletsAcrossATwoColumnLayout(t *testing.T) {
	manifest := testManifest()
	source := "# 비교\n@two\n- 첫째\n  - 근거\n- 둘째\n- 셋째\n- 넷째\n"
	result := Compile(ParseSource(source), manifest, CompileOptions{Language: "ko"})
	content := Decode(result.Slides[0].Content)
	if len(content.Fields[pptx.SlotBody]) == 0 || len(content.Fields["body2"]) == 0 {
		t.Fatalf("both columns should be used: %+v", content.Fields)
	}
	// A sub-bullet stays with the point it belongs to.
	first := content.Fields[pptx.SlotBody]
	if first[0].Text != "첫째" || first[1].Level != 1 {
		t.Fatalf("first column = %+v", first)
	}
}

func TestCompileReportsAMissingLayoutInsteadOfFailing(t *testing.T) {
	manifest := testManifest()
	result := Compile(ParseSource("# 제목\n@layout nonexistent\n- 내용\n"), manifest, CompileOptions{})
	if len(result.Slides) != 1 {
		t.Fatalf("the slide should still be produced: %+v", result)
	}
	if len(result.Warnings) == 0 || !strings.Contains(strings.Join(result.Warnings, " "), "nonexistent") {
		t.Fatalf("warnings = %v", result.Warnings)
	}
}

func TestFormatRoundTripsThroughCompile(t *testing.T) {
	manifest := testManifest()
	first := Compile(ParseSource(sample), manifest, CompileOptions{Language: "ko"})
	presentation := model.Presentation{Slides: first.Slides}
	formatted := Format(presentation, manifest)
	second := Compile(ParseSource(formatted), manifest, CompileOptions{Language: "ko"})

	if len(second.Slides) != len(first.Slides) {
		t.Fatalf("round trip changed the slide count: %d then %d\n%s",
			len(first.Slides), len(second.Slides), formatted)
	}
	for index := range first.Slides {
		if first.Slides[index].Title != second.Slides[index].Title {
			t.Fatalf("slide %d title changed: %q then %q", index+1,
				first.Slides[index].Title, second.Slides[index].Title)
		}
		if first.Slides[index].LayoutID != second.Slides[index].LayoutID {
			t.Fatalf("slide %d layout changed: %q then %q\n%s", index+1,
				first.Slides[index].LayoutID, second.Slides[index].LayoutID, formatted)
		}
		before, after := Decode(first.Slides[index].Content), Decode(second.Slides[index].Content)
		if len(before.Blocks) != len(after.Blocks) {
			t.Fatalf("slide %d components changed: %d then %d\n%s", index+1,
				len(before.Blocks), len(after.Blocks), formatted)
		}
		if len(before.Fields[pptx.SlotBody]) != len(after.Fields[pptx.SlotBody]) {
			t.Fatalf("slide %d body changed:\n%+v\n%+v\n%s", index+1,
				before.Fields[pptx.SlotBody], after.Fields[pptx.SlotBody], formatted)
		}
	}
}

// TestFormatIsDeterministic runs Format repeatedly: a map iteration in the middle
// of it would make the output vary.
func TestFormatIsDeterministic(t *testing.T) {
	manifest := testManifest()
	first := Compile(ParseSource(sample), manifest, CompileOptions{Language: "ko"})
	presentation := model.Presentation{Slides: first.Slides}
	baseline := Format(presentation, manifest)
	for attempt := 0; attempt < 40; attempt++ {
		if got := Format(presentation, manifest); got != baseline {
			t.Fatalf("Format is not deterministic. attempt %d differs:\n--- baseline\n%s\n--- got\n%s",
				attempt, baseline, got)
		}
	}
}

// TestFormatEscapesWithoutAlteringText checks that escaping a directive-like line
// does not smuggle characters into the deck's text.
func TestFormatEscapesWithoutAlteringText(t *testing.T) {
	manifest := testManifest()
	content := Content{Type: ContentType, LayoutID: "content"}
	content.SetField(pptx.SlotTitle, []pptx.Paragraph{{Text: "- 대시로 시작하는 제목"}})
	content.SetField(pptx.SlotBody, []pptx.Paragraph{{Text: "@역할처럼 보이는 문장"}})
	presentation := model.Presentation{Slides: []model.Slide{{Position: 1, Title: "- 대시로 시작하는 제목",
		Content: content.Encode(), Layout: pptx.RoleContent, LayoutID: "content"}}}
	formatted := Format(presentation, manifest)
	round := Compile(ParseSource(formatted), manifest, CompileOptions{})
	after := Decode(round.Slides[0].Content)
	title := after.Fields[pptx.SlotTitle][0].Text
	if title != "- 대시로 시작하는 제목" {
		t.Fatalf("the title changed through a round trip: %q (bytes %v)", title, []byte(title))
	}
	body := after.Fields[pptx.SlotBody][0].Text
	if body != "@역할처럼 보이는 문장" {
		t.Fatalf("the body changed through a round trip: %q (bytes %v)", body, []byte(body))
	}
	for _, text := range []string{title, body} {
		if strings.ContainsRune(text, '​') {
			t.Fatalf("an invisible character was inserted into %q", text)
		}
	}
}

func TestCompileBuildsATableFromItsRows(t *testing.T) {
	manifest := testManifest()
	source := "# 비용 비교\n::table 연간 비용\n- 항목 | 2026 | 2027\n- 인건비 | 4.2억 | 3.4억\n- 라이선스 | 1.1억 | 1.4억\n::\n"
	result := Compile(ParseSource(source), manifest, CompileOptions{Language: "ko"})
	content := Decode(result.Slides[0].Content)
	block, ok := content.Blocks[pptx.SlotBody]
	if !ok {
		t.Fatalf("the table was not bound: %+v (warnings %v)", content, result.Warnings)
	}
	if len(block.Columns) != 3 || block.Columns[2] != "2027" {
		t.Fatalf("columns = %v", block.Columns)
	}
	if len(block.Rows) != 2 || block.Rows[1][0] != "라이선스" {
		t.Fatalf("rows = %v", block.Rows)
	}
	// A table's header must not also appear as an item.
	if len(block.Items) != 0 {
		t.Fatalf("items should be empty for a table: %+v", block.Items)
	}
}

func TestCompileBuildsLineSeriesFromRows(t *testing.T) {
	manifest := testManifest()
	source := "# 추이\n::line 월별 처리량\n- 월 | 1월, 2월, 3월, 4월\n- 전환 전 | 120, 118, 121, 119\n- 전환 후 | 120, 132, 148, 165\n::\n"
	result := Compile(ParseSource(source), manifest, CompileOptions{Language: "ko"})
	block := Decode(result.Slides[0].Content).Blocks[pptx.SlotBody]
	if len(block.Series) != 2 {
		t.Fatalf("series = %+v (warnings %v)", block.Series, result.Warnings)
	}
	if block.Series[1].Name != "전환 후" || len(block.Series[1].Points) != 4 || block.Series[1].Points[3] != 165 {
		t.Fatalf("second series = %+v", block.Series[1])
	}
	// A row of words is the axis, not a series.
	if len(block.Labels) != 4 || block.Labels[0] != "1월" {
		t.Fatalf("labels = %v", block.Labels)
	}
}

func TestCompileWarningsPointAtTheLine(t *testing.T) {
	manifest := testManifest()
	source := "# 제목\n@layout nonexistent\n- 내용\n"
	result := Compile(ParseSource(source), manifest, CompileOptions{})
	joined := strings.Join(result.Warnings, "\n")
	if !strings.Contains(joined, "line 1") {
		t.Fatalf("a warning should name the line it belongs to: %v", result.Warnings)
	}
}

func TestParseSourceKeepsAPipeInsideAField(t *testing.T) {
	parsed := ParseSource("# 제목\n::kpi\n- A \\| B | 42개\n::\n")
	if len(parsed.Slides) != 1 || len(parsed.Slides[0].Blocks) != 1 {
		t.Fatalf("parsed = %+v", parsed)
	}
	item := parsed.Slides[0].Blocks[0].Items[0]
	if item.Label != "A | B" || item.Value != "42개" {
		t.Fatalf("item = %+v", item)
	}
}

func TestCompileBindsAnImageToItsSlot(t *testing.T) {
	manifest := testManifest()
	// A layout with a picture region: an image belongs where the designer put one.
	manifest.Layouts = append(manifest.Layouts, pptx.Layout{
		ID: "picture", Name: "Picture", Role: pptx.RolePicture,
		Placeholders: []pptx.Placeholder{
			{Slot: pptx.SlotTitle, Kind: "text", Type: "title", Width: 8000000, Height: 900000, FontSize: 3200, MaxChars: 60, MaxLines: 2},
			{Slot: pptx.SlotPicture, Kind: "picture", Type: "pic", X: 6000000, Y: 1500000, Width: 5000000, Height: 3500000},
		},
	})
	resolved := map[string]string{"로고": "asset-1"}
	options := CompileOptions{Language: "ko", ResolveImage: func(reference string) (ContentImage, bool) {
		id, ok := resolved[reference]
		return ContentImage{AssetID: id, Name: reference}, ok
	}}

	result := Compile(ParseSource("# 브랜드\n@picture\n::image 로고 | 2026 브랜드 마크\n"), manifest, options)
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings = %v", result.Warnings)
	}
	content := Decode(result.Slides[0].Content)
	placed, ok := content.Images[pptx.SlotPicture]
	if !ok {
		t.Fatalf("the image was not bound to the picture region: %+v", content.Images)
	}
	if placed.AssetID != "asset-1" || placed.Caption != "2026 브랜드 마크" {
		t.Fatalf("image = %+v", placed)
	}

	// A name nobody uploaded is reported with its line rather than dropped.
	missing := Compile(ParseSource("# 브랜드\n::image 없는이름\n"), manifest, options)
	if len(missing.Warnings) == 0 || !strings.Contains(strings.Join(missing.Warnings, " "), "없는이름") {
		t.Fatalf("a missing image must be reported: %v", missing.Warnings)
	}

	// Without a resolver the directive is reported, never silently ignored.
	unresolved := Compile(ParseSource("# 브랜드\n::image 로고\n"), manifest, CompileOptions{})
	if len(unresolved.Warnings) == 0 {
		t.Fatal("an image with no way to resolve it must be reported")
	}

	// The image survives a round trip through the text, by the name it was written
	// with rather than by an id nobody typed.
	presentation := model.Presentation{Slides: result.Slides}
	formatted := Format(presentation, manifest)
	if !strings.Contains(formatted, "::image 로고 | 2026 브랜드 마크") {
		t.Fatalf("the image was not written back out:\n%s", formatted)
	}
	again := Compile(ParseSource(formatted), manifest, options)
	if len(Decode(again.Slides[0].Content).Images) != 1 {
		t.Fatalf("the image was lost on the round trip:\n%s", formatted)
	}
}

func TestCompileDoesNotRepeatTheLeadLine(t *testing.T) {
	manifest := testManifest()
	// A layout with a subtitle region and a body region: the lead belongs in one
	// of them, and recording it as the slide's subtitle as well made the renderer
	// write it into both.
	result := Compile(ParseSource("# 제목\n> 한 줄 리드\n- 요점\n"), manifest, CompileOptions{Language: "ko"})
	slide := result.Slides[0]
	content := Decode(slide.Content)
	if len(content.Fields[pptx.SlotSubtitle]) == 1 && slide.Subtitle == "" {
		t.Fatal("a lead in the subtitle region should also be the slide's subtitle")
	}

	// A layout without a subtitle region: the lead goes to the body, and the slide
	// carries no subtitle for a renderer to duplicate.
	twoColumn := Compile(ParseSource("# 제목\n@two\n> 한 줄 리드\n- 요점\n"), manifest, CompileOptions{Language: "ko"})
	body := Decode(twoColumn.Slides[0].Content)
	if _, ok := body.Fields[pptx.SlotSubtitle]; ok {
		t.Fatalf("this layout has no subtitle region: %+v", body.Fields)
	}
	if twoColumn.Slides[0].Subtitle != "" {
		t.Fatalf("subtitle = %q; the lead is in the body, so nothing may repeat it", twoColumn.Slides[0].Subtitle)
	}
	// The text is present exactly once.
	occurrences := 0
	for _, paragraphs := range body.Fields {
		for _, paragraph := range paragraphs {
			if strings.Contains(paragraph.Text, "한 줄 리드") {
				occurrences++
			}
		}
	}
	if occurrences != 1 {
		t.Fatalf("the lead appears %d times: %+v", occurrences, body.Fields)
	}
}

func TestCompileReportsTextItCouldNotFit(t *testing.T) {
	manifest := testManifest()
	long := strings.Repeat("전환 대상 시스템의 이관 순서와 선행 조건을 한 문장에 담아 설명하는 긴 문장입니다. ", 3)
	var builder strings.Builder
	builder.WriteString("# 과적재\n")
	for range 8 {
		builder.WriteString("- " + long + "\n")
	}
	result := Compile(ParseSource(builder.String()), manifest, CompileOptions{Language: "ko"})
	joined := strings.Join(result.Warnings, "\n")
	// Dropping a point silently is the one behaviour this must not have.
	if !strings.Contains(joined, "did not fit") {
		t.Fatalf("text that was left out must be reported: %v", result.Warnings)
	}
	if !strings.Contains(joined, "line 1") {
		t.Fatalf("the report should name the line: %v", result.Warnings)
	}
	// A slide that fits says nothing.
	quiet := Compile(ParseSource("# 제목\n- 짧은 요점\n- 두 번째 요점\n"), manifest, CompileOptions{Language: "ko"})
	if len(quiet.Warnings) != 0 {
		t.Fatalf("a slide that fits must be quiet: %v", quiet.Warnings)
	}
}
