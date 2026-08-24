package deck

import (
	"fmt"
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
	// The bullets that follow a component need a region of their own, so the
	// compiler chooses the layout that has one. A slide is bound to the layout
	// that can hold it, not merely to the layout its role names.
	if len(content.Fields[pptx.SlotBody]) > 0 {
		t.Fatal("prose and a component must not share one slot")
	}
	if len(content.Fields["body2"]) == 0 {
		t.Fatalf("the prose should have a region of its own: %+v", content.Fields)
	}
	if strings.TrimSpace(content.Body) != "" || len(result.Warnings) != 0 {
		t.Fatalf("nothing should have been left over: %q %v", content.Body, result.Warnings)
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

// A deck's shape is not negotiable — the last slide is the closing — but a
// template with more than one closing page should be given the one that can hold
// the ask. "다음 단계" with three requests set as a stray line under the title is
// the most important slide of the deck being dropped on the floor.
func TestAClosingSlideKeepsItsPoints(t *testing.T) {
	manifest := testManifest()
	statement := pptx.Layout{ID: "마무리-문구", Name: "마무리 문구", Role: pptx.RoleClosing,
		Placeholders: []pptx.Placeholder{
			{Slot: pptx.SlotTitle, Kind: "text", Type: "title", X: 900000, Y: 2000000, Width: 8000000, Height: 900000, MaxChars: 40, MaxLines: 2},
		}}
	holds := pptx.Layout{ID: "마무리-목록", Name: "마무리 목록", Role: pptx.RoleClosing,
		Placeholders: []pptx.Placeholder{
			{Slot: pptx.SlotTitle, Kind: "text", Type: "title", X: 900000, Y: 800000, Width: 8000000, Height: 900000, MaxChars: 40, MaxLines: 2},
			{Slot: pptx.SlotBody, Kind: "text", Type: "body", X: 900000, Y: 1900000, Width: 8000000, Height: 2600000, MaxChars: 60, MaxLines: 6},
		}}
	manifest.Layouts = append([]pptx.Layout{statement, holds}, manifest.Layouts...)

	source := "# 표지\n@cover\n> 여는 줄\n\n# 본문\n- 한 줄\n\n# 다음 단계\n@closing\n> 결정과 실행을 나눠 요청합니다\n- 오늘 요청하는 결정\n- 30일 안에 진행할 일\n- 다음 보고 시점\n"
	compiled := Compile(ParseSource(source), manifest, CompileOptions{Language: "ko"})
	last := compiled.Slides[len(compiled.Slides)-1]
	if last.Layout != pptx.RoleClosing {
		t.Fatalf("the deck stopped closing on a closing layout: %q", last.Layout)
	}
	if last.LayoutID != holds.ID {
		t.Fatalf("the closing slide landed on %q, which has no room for its points", last.LayoutID)
	}
	content := Decode(last.Content)
	if len(content.Fields[pptx.SlotBody]) != 3 {
		t.Fatalf("the closing slide kept %d of its three points: %+v", len(content.Fields[pptx.SlotBody]), content.Fields)
	}
}

// A template without a layout for a role falls back to a neighbouring one, and a
// neighbour designed for something else can read badly: Microsoft's own Section
// Header puts its title below its body, which turns a closing slide with three
// requests upside down. A slide that carries points goes to a content layout.
func TestAMissingRoleSendsPointsToAContentLayout(t *testing.T) {
	manifest := testManifest()
	// A template like the Office default: a section header, no closing layout.
	sectionHeader := pptx.Layout{ID: "section-header", Name: "Section Header", Role: pptx.RoleSection,
		Placeholders: []pptx.Placeholder{
			{Slot: pptx.SlotTitle, Kind: "text", Type: "title", X: 800000, Y: 3400000, Width: 8000000, Height: 1200000, MaxChars: 40, MaxLines: 2},
			{Slot: pptx.SlotBody, Kind: "text", Type: "body", X: 800000, Y: 1400000, Width: 8000000, Height: 1800000, MaxChars: 60, MaxLines: 4},
		}}
	kept := make([]pptx.Layout, 0, len(manifest.Layouts)+1)
	kept = append(kept, sectionHeader)
	for _, layout := range manifest.Layouts {
		if layout.Role != pptx.RoleClosing {
			kept = append(kept, layout)
		}
	}
	manifest.Layouts = kept

	source := "# 표지\n@cover\n> 여는 줄\n\n# 본문\n- 한 줄\n\n# 다음 단계\n@closing\n- 오늘 요청하는 결정\n- 30일 안에 진행할 일\n- 다음 보고 시점\n"
	compiled := Compile(ParseSource(source), manifest, CompileOptions{Language: "ko"})
	last := compiled.Slides[len(compiled.Slides)-1]
	if last.LayoutID == sectionHeader.ID {
		t.Fatalf("the closing slide landed on the section header, whose title sits under its body")
	}
	if layout, ok := manifest.Layout(last.LayoutID); !ok || layout.Role != pptx.RoleContent {
		t.Fatalf("the closing slide landed on %q", last.LayoutID)
	}
	if len(Decode(last.Content).Fields[pptx.SlotBody]) != 3 {
		t.Fatalf("the closing slide lost its points: %+v", Decode(last.Content).Fields)
	}
	// A closing slide with nothing but a line keeps the design's own page.
	statement := Compile(ParseSource("# 표지\n@cover\n> 여는 줄\n\n# 본문\n- 한 줄\n\n# 감사합니다\n@closing\n> 질문 받겠습니다\n"),
		manifest, CompileOptions{Language: "ko"})
	if final := statement.Slides[len(statement.Slides)-1]; final.LayoutID != sectionHeader.ID {
		t.Fatalf("a closing slide with no points should keep the design's page, got %q", final.LayoutID)
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

// A layout with no subtitle slot has the slide's lead folded into a component's
// heading. Writing the deck back out has to put it somewhere, or opening the
// source and applying it deletes a line the author wrote.
func TestFormatKeepsALeadFoldedIntoAComponent(t *testing.T) {
	manifest := testManifest()
	source := "# 회사 소개\n@two\n> 2015년 설립, 임직원 240명\n::kpi 한눈에\n- 설립 | 2015\n- 임직원 | 240명\n::\n- 클라우드 전환 전문\n"
	first := Compile(ParseSource(source), manifest, CompileOptions{Language: "ko"})
	if len(first.Slides) != 1 {
		t.Fatalf("compiled %d slides", len(first.Slides))
	}
	// The compiler had nowhere to put the lead but the component's heading.
	content := Decode(first.Slides[0].Content)
	heading := ""
	for _, block := range content.Blocks {
		heading = strings.TrimSpace(block.Heading)
	}
	if heading != "2015년 설립, 임직원 240명" {
		t.Skipf("this template keeps the lead elsewhere (%q); nothing to round trip", heading)
	}
	formatted := Format(model.Presentation{Slides: first.Slides}, manifest)
	if !strings.Contains(formatted, "> 2015년 설립, 임직원 240명") {
		t.Fatalf("the lead was lost writing the deck back out:\n%s", formatted)
	}
	second := Compile(ParseSource(formatted), manifest, CompileOptions{Language: "ko"})
	after := Decode(second.Slides[0].Content)
	for _, block := range after.Blocks {
		if strings.TrimSpace(block.Heading) != heading {
			t.Fatalf("the lead did not survive a round trip: %q", block.Heading)
		}
		if strings.TrimSpace(block.Caption) != "한눈에" {
			t.Fatalf("the component's caption changed: %q", block.Caption)
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
	// Distinct lines: a slide that repeats itself has its echoes removed before
	// anything is measured, which is a different behaviour with its own test.
	for index := range 8 {
		builder.WriteString(fmt.Sprintf("- %d번째 항목. %s\n", index+1, long))
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

func TestCompileDrawsAGridFromItsDefinition(t *testing.T) {
	manifest := testManifest()
	source := "# 담당 체계\n::grid raci 전환 프로젝트\n- 활동 | 기획 | 개발 | 운영\n- 요건 정의 | R | C | I\n- 설계 | A | R | C\n::\n"
	result := Compile(ParseSource(source), manifest, CompileOptions{Language: "ko"})
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings = %v", result.Warnings)
	}
	block := Decode(result.Slides[0].Content).Blocks[pptx.SlotBody]
	if block.Kind != pptx.BlockGrid || block.Grid == nil {
		t.Fatalf("block = %+v", block)
	}
	// The shipped definition is found without anything being configured.
	if block.Grid.Name != "raci" || len(block.Grid.Values) == 0 {
		t.Fatalf("definition = %+v", block.Grid)
	}
	// The first row is the header; the rest are data.
	if len(block.Columns) != 4 || block.Columns[1] != "기획" {
		t.Fatalf("columns = %v", block.Columns)
	}
	if len(block.Rows) != 2 || block.Rows[1][0] != "설계" {
		t.Fatalf("rows = %v", block.Rows)
	}
	if block.Caption != "전환 프로젝트" {
		t.Fatalf("caption = %q", block.Caption)
	}

	// A definition nobody wrote is reported with its line, and the rows are kept
	// as text rather than vanishing.
	missing := Compile(ParseSource("# 표\n::grid nonexistent\n- 항목 | 값\n::\n"), manifest, CompileOptions{})
	if len(missing.Warnings) == 0 || !strings.Contains(strings.Join(missing.Warnings, " "), "nonexistent") {
		t.Fatalf("warnings = %v", missing.Warnings)
	}
	if content := Decode(missing.Slides[0].Content); len(content.Fields[pptx.SlotBody]) == 0 && strings.TrimSpace(content.Body) == "" {
		t.Fatalf("the rows should survive as text: %+v", content)
	}

	// A deployment's own definition shadows the shipped one.
	custom := pptx.GridSpec{Name: "raci", Title: "우리 회사 RACI",
		Values: map[string]pptx.GridValue{"R": {Label: "실행", Role: "accent2", Chip: true}}}
	overridden := Compile(ParseSource(source), manifest, CompileOptions{
		ResolveGrid: func(name string) (pptx.GridSpec, bool) {
			if name == "raci" {
				return custom, true
			}
			return pptx.GridSpec{}, false
		}})
	if title := Decode(overridden.Slides[0].Content).Blocks[pptx.SlotBody].Grid.Title; title != "우리 회사 RACI" {
		t.Fatalf("the deployment's own definition should win, got %q", title)
	}

	// The grid survives a round trip through the text, definition name and all.
	formatted := Format(model.Presentation{Slides: result.Slides}, manifest)
	if !strings.Contains(formatted, "::grid raci") || !strings.Contains(formatted, "- 요건 정의 | R | C | I") {
		t.Fatalf("the grid was not written back out:\n%s", formatted)
	}
	again := Compile(ParseSource(formatted), manifest, CompileOptions{Language: "ko"})
	if block := Decode(again.Slides[0].Content).Blocks[pptx.SlotBody]; block.Grid == nil || len(block.Rows) != 2 {
		t.Fatalf("the grid was lost on the round trip:\n%s", formatted)
	}
}

func TestCompileAcceptsALayoutNamedLoosely(t *testing.T) {
	manifest := testManifest()
	manifest.Layouts[2].Name = "콘텐츠 2개"
	manifest.Layouts[2].ID = "콘텐츠-2개"
	// A model copies a layout's name out of the catalogue as often as its id, and
	// spells it as it reads: "콘텐츠 2개" for "콘텐츠-2개".
	for _, reference := range []string{"콘텐츠-2개", "콘텐츠 2개", "콘텐츠  2개"} {
		source := "# 비교\n@layout " + reference + "\n- 첫째\n- 둘째\n"
		result := Compile(ParseSource(source), manifest, CompileOptions{Language: "ko"})
		if got := result.Slides[0].LayoutID; got != "콘텐츠-2개" {
			t.Fatalf("@layout %q resolved to %q", reference, got)
		}
		if len(result.Warnings) != 0 {
			t.Fatalf("@layout %q warned: %v", reference, result.Warnings)
		}
	}
	// A name that matches nothing is still reported rather than guessed at.
	result := Compile(ParseSource("# 비교\n@layout 없는 레이아웃\n- 첫째\n"), manifest, CompileOptions{})
	if len(result.Warnings) == 0 {
		t.Fatal("an unknown layout must be reported")
	}
}

func TestParseSourceReadsWhatAModelActuallyWrites(t *testing.T) {
	// Every mistake below was made by a real 122B model on the first attempt:
	// a title with no hash, a component closed before its rows, markdown pipes,
	// and a space between a number and its Korean unit.
	source := "리스크 대응 전략\n@content\n> 3 년간 관리합니다.\n::comparison\n::\n" +
		"| 기존 방식 | 점진적 전환 |\n| 신규 방식 | 병렬 운영 |\n\n" +
		"# 비용\n::kpi 비용\n- 기존 | 120 억 원\n- 신규 | 72 억 원\n::\n"
	parsed := ParseSource(source)
	if len(parsed.Slides) != 2 {
		t.Fatalf("slides = %d: %+v", len(parsed.Slides), parsed.Slides)
	}

	// The bare line before @content was the title, not a lead.
	first := parsed.Slides[0]
	if first.Title != "리스크 대응 전략" || first.Lead == first.Title {
		t.Fatalf("first slide = %+v", first)
	}
	// A number keeps its unit and its particle: "3 년간" is not Korean.
	if first.Lead != "3년간 관리합니다." {
		t.Fatalf("lead = %q", first.Lead)
	}
	// The rows written after :: still belong to the component, and the markdown
	// pipes do not turn a two-column row into one nameless entry.
	if len(first.Blocks) != 1 || len(first.Blocks[0].Items) != 2 {
		t.Fatalf("blocks = %+v", first.Blocks)
	}
	if item := first.Blocks[0].Items[0]; item.Label != "기존 방식" || item.Value != "점진적 전환" {
		t.Fatalf("first row = %+v", item)
	}
	if rows := first.Blocks[0].Rows; len(rows) != 2 || len(rows[0]) != 2 {
		t.Fatalf("rows = %v", rows)
	}
	// The second slide's figures are tidied too.
	if item := parsed.Slides[1].Blocks[0].Items[0]; item.Value != "120억 원" {
		t.Fatalf("kpi value = %q", item.Value)
	}
}

func TestParseSourcePromotesPipeRowsIntoAComponent(t *testing.T) {
	// Asked for a comparison table, the model wrote the rows as plain bullets.
	// Drawn as prose the pipes are noise; drawn as a component they are a table.
	parsed := ParseSource("# 기존 방식과 신규 방식 비교\n@content\n> 아키텍처가 다릅니다.\n" +
		"- 단일 서버 의존 | 마이크로서비스 분산\n- 수동 배포 | 자동화된 CI/CD\n- 확장성 제한 | 탄력적 자동 확장\n")
	slide := parsed.Slides[0]
	if len(slide.Blocks) != 1 {
		t.Fatalf("expected a component, got blocks=%+v bullets=%+v", slide.Blocks, slide.Bullets)
	}
	if slide.Blocks[0].Kind != "comparison" {
		t.Fatalf("kind = %q", slide.Blocks[0].Kind)
	}
	if len(slide.Blocks[0].Rows) != 3 || slide.Blocks[0].Rows[0][1] != "마이크로서비스 분산" {
		t.Fatalf("rows = %v", slide.Blocks[0].Rows)
	}
	// The prose around the table stays prose.
	if slide.Lead != "아키텍처가 다릅니다." || len(slide.Bullets) != 0 {
		t.Fatalf("lead = %q bullets = %+v", slide.Lead, slide.Bullets)
	}

	// A markdown table, separator rule and all, is the same thing.
	markdown := ParseSource("# 비교\n@content\n| 항목 | 결과 |\n|---|---|\n| 응답 시간 | 240ms |\n| 오류율 | 0.2% |\n")
	if blocks := markdown.Slides[0].Blocks; len(blocks) != 1 || len(blocks[0].Rows) != 3 {
		t.Fatalf("markdown blocks = %+v", blocks)
	}

	// Two columns of figures are indicators, which read better than a table.
	figures := ParseSource("# 지표\n@content\n- 절감액 | 48억 원\n- 기간 | 6개월\n")
	if blocks := figures.Slides[0].Blocks; len(blocks) != 1 || blocks[0].Kind != "kpi" {
		t.Fatalf("figure blocks = %+v", blocks)
	}

	// One pipe on its own is a sentence, not a table.
	prose := ParseSource("# 균형\n@content\n- 가격 | 성능의 균형이 핵심입니다\n- 그리고 속도도 중요합니다\n")
	if len(prose.Slides[0].Blocks) != 0 || len(prose.Slides[0].Bullets) != 2 {
		t.Fatalf("a lone pipe must stay prose: %+v", prose.Slides[0])
	}

	// Promotion produces exactly what the component would have written, so a
	// promoted deck and an explicit one compile identically.
	explicit := ParseSource("# 기존 방식과 신규 방식 비교\n@content\n> 아키텍처가 다릅니다.\n" +
		"::comparison\n- 단일 서버 의존 | 마이크로서비스 분산\n- 수동 배포 | 자동화된 CI/CD\n- 확장성 제한 | 탄력적 자동 확장\n::\n")
	if fmt.Sprint(explicit.Slides[0].Blocks[0].Rows) != fmt.Sprint(slide.Blocks[0].Rows) {
		t.Fatalf("promoted rows differ from written rows:\n%v\n%v", slide.Blocks[0].Rows, explicit.Slides[0].Blocks[0].Rows)
	}
}

// A model names a layout the way an API would: "@layout id=제목-및-내용". Read
// literally that names no layout at all, and the slide quietly moves to another
// one — the failure the compiler's own warnings caught on a real deck.
func TestParseSourceReadsALayoutWrittenAsAnAssignment(t *testing.T) {
	for _, written := range []string{"제목-및-내용", "id=제목-및-내용", "id: 제목-및-내용", `name="제목-및-내용"`} {
		parsed := ParseSource("# 현황\n@layout " + written + "\n- 요점\n")
		if got := parsed.Slides[0].LayoutID; got != "제목-및-내용" {
			t.Fatalf("@layout %s -> %q", written, got)
		}
		if len(parsed.Warnings) != 0 {
			t.Fatalf("@layout %s warned: %v", written, parsed.Warnings)
		}
	}
}

// A slide is bound to the layout that can hold what it carries. Choosing on the
// declared role alone is how a component lands in a caption strip and the
// author's points get dropped.
func TestCompileChoosesTheLayoutThatFits(t *testing.T) {
	manifest := testManifest()

	// A component and four points need two regions.
	both := Compile(ParseSource("# 이행 계획\n::steps\n- 준비 | 범위 확정\n- 이행 | 이관\n- 안정화 | 점검\n::\n"+
		"- 첫 번째 근거를 한 문장으로 적습니다\n- 두 번째 근거를 한 문장으로 적습니다\n"+
		"- 세 번째 근거를 한 문장으로 적습니다\n"), manifest, CompileOptions{Language: "ko"})
	if len(both.Slides) != 1 {
		t.Fatalf("slides = %d", len(both.Slides))
	}
	if both.Slides[0].LayoutID != "two" {
		t.Fatalf("a component beside prose needs two regions, got %q", both.Slides[0].LayoutID)
	}
	if len(both.Warnings) != 0 {
		t.Fatalf("nothing should be lost: %v", both.Warnings)
	}

	// Prose alone belongs on the plain content layout, not in a two-column one
	// with a column left staring back at the room.
	prose := Compile(ParseSource("# 하나의 논지\n- 짧은 요점 하나\n- 짧은 요점 둘\n"),
		manifest, CompileOptions{Language: "ko"})
	if prose.Slides[0].LayoutID != "content" {
		t.Fatalf("prose alone belongs on the content layout, got %q", prose.Slides[0].LayoutID)
	}

	// An author who names a layout gets it, fit or no fit.
	named := Compile(ParseSource("# 고른 대로\n@layout two\n- 한 줄\n"), manifest, CompileOptions{Language: "ko"})
	if named.Slides[0].LayoutID != "two" {
		t.Fatalf("an explicit layout must win, got %q", named.Slides[0].LayoutID)
	}
}

// A model transcribing a layout id sometimes adds a space. Refusing the near
// miss costs the slide its layout, and the fallback used to ignore what the
// slide was carrying.
func TestCompileAcceptsANearMissLayoutName(t *testing.T) {
	manifest := testManifest()
	result := Compile(ParseSource("# 두 갈래\n@layout Two content\n- 왼쪽 이야기\n- 오른쪽 이야기\n"),
		manifest, CompileOptions{Language: "ko"})
	if result.Slides[0].LayoutID != "two" {
		t.Fatalf("a layout named with a space should still resolve: %q", result.Slides[0].LayoutID)
	}
	// A name that names nothing falls through to the layout that fits.
	missing := Compile(ParseSource("# 없는 이름\n@layout nowhere\n::steps\n- 준비 | 범위\n- 이행 | 이관\n- 안정화 | 점검\n::\n"+
		"- 이 슬라이드의 근거를 한 문장으로 적습니다\n- 두 번째 근거도 한 문장으로 적습니다\n"),
		manifest, CompileOptions{Language: "ko"})
	if missing.Slides[0].LayoutID != "two" {
		t.Fatalf("an unknown layout should fall back to one that fits, got %q", missing.Slides[0].LayoutID)
	}
	if len(missing.Warnings) == 0 {
		t.Fatal("substituting a layout has to be reported")
	}
}

// A layout named by a person is a decision; one named by the model is a guess it
// made before it knew what the slide would carry.
func TestAModelsLayoutChoiceIsASuggestion(t *testing.T) {
	manifest := testManifest()
	source := "# 이행 계획\n@layout content\n::steps\n- 준비 | 범위 확정\n- 이행 | 이관\n- 안정화 | 점검\n::\n" +
		"- 첫 번째 근거를 한 문장으로 적습니다\n- 두 번째 근거를 한 문장으로 적습니다\n"

	// Written by a person: their choice stands, whatever it costs.
	byHand := Compile(ParseSource(source), manifest, CompileOptions{Language: "ko"})
	if byHand.Slides[0].LayoutID != "content" {
		t.Fatalf("a person's layout must stand, got %q", byHand.Slides[0].LayoutID)
	}

	// Written by the model: the slide moves to a layout that can hold it.
	generated := Compile(ParseSource(source), manifest, CompileOptions{Language: "ko", LayoutsAreSuggestions: true})
	if generated.Slides[0].LayoutID != "two" {
		t.Fatalf("a model's layout should give way to one that fits, got %q", generated.Slides[0].LayoutID)
	}
	if len(generated.Warnings) == 0 {
		t.Fatal("moving a slide has to be reported")
	}
	// A suggestion that already fits is left alone.
	fine := Compile(ParseSource("# 하나\n@layout two\n- 왼쪽\n- 오른쪽\n"),
		manifest, CompileOptions{Language: "ko", LayoutsAreSuggestions: true})
	if fine.Slides[0].LayoutID != "two" {
		t.Fatalf("a workable suggestion should be kept, got %q", fine.Slides[0].LayoutID)
	}
}

// A slide that repeats its own lead as a bullet says the same sentence twice on
// the page. The model does it often enough to be worth removing at compile time.
func TestCompileDropsAPointThatOnlyRepeatsTheLead(t *testing.T) {
	manifest := testManifest()
	result := Compile(ParseSource("# 병목\n> 수작업 병목이 2026년 성장의 최대 제약입니다\n"+
		"- 수작업 병목이 2026 년 성장의 최대 제약입니다.\n- 처리 시간은 건당 4시간입니다\n"+
		"- 처리 시간은 건당 4시간입니다\n"), manifest, CompileOptions{Language: "ko"})
	content := Decode(result.Slides[0].Content)
	lines := 0
	for slot, paragraphs := range content.Fields {
		if slot == pptx.SlotTitle || slot == pptx.SlotSubtitle {
			continue
		}
		lines += len(paragraphs)
	}
	if lines != 1 {
		t.Fatalf("the echo of the lead and the duplicate point should both go: %+v", content.Fields)
	}
}

// A model writing Korean leaves a space between a figure and its unit and
// between a foreign word and its particle. The slides were already tidied as
// they were parsed — except the speaker notes, which are the one line the tidy
// never reached.
func TestSpeakerNotesAreTidiedToo(t *testing.T) {
	parsed := ParseSource("# 1 단계 및 2 단계\n> 2026 년 상반기\n- 현재 4 시간 지연\n" +
		"!notes 1 단계는 4 시간입니다. deliverables 를 94% 의 정확도로 확인합니다.\n")
	if len(parsed.Slides) != 1 {
		t.Fatalf("parsed %d slides", len(parsed.Slides))
	}
	slide := parsed.Slides[0]
	if slide.Notes != "1단계는 4시간입니다. deliverables를 94%의 정확도로 확인합니다." {
		t.Errorf("the notes were not tidied: %q", slide.Notes)
	}
	if slide.Title != "1단계 및 2단계" || slide.Lead != "2026년 상반기" {
		t.Errorf("title = %q, lead = %q", slide.Title, slide.Lead)
	}
}

// A lead is the slide's one sentence, not one of its points. A layout with no
// subtitle region keeps it at the head of the body — and it used to arrive there
// with a bullet in front of it, reading as the first point, and came back out of
// a round trip as one.
func TestALeadInTheBodyStaysALead(t *testing.T) {
	manifest := testManifest()
	source := "# 제목\n@two\n> 한 줄 리드\n- 첫 요점\n- 두 번째 요점\n"
	result := Compile(ParseSource(source), manifest, CompileOptions{Language: "ko"})
	content := Decode(result.Slides[0].Content)
	found := false
	for _, paragraphs := range content.Fields {
		for index, paragraph := range paragraphs {
			if paragraph.Text != "한 줄 리드" {
				continue
			}
			found = true
			if !paragraph.Lead {
				t.Error("the lead is not marked as one, so it is drawn as a point")
			}
			if index != 0 {
				t.Errorf("the lead is at position %d of its region", index)
			}
		}
	}
	if !found {
		t.Fatalf("the lead is not on the slide: %+v", content.Fields)
	}

	// And it comes back out as the lead.
	presentation := model.Presentation{Language: "ko", Slides: result.Slides}
	written := Format(presentation, manifest)
	if !strings.Contains(written, "> 한 줄 리드\n") {
		t.Errorf("the lead came back as something else:\n%s", written)
	}
	if strings.Contains(written, "- 한 줄 리드") {
		t.Errorf("the lead came back as a point:\n%s", written)
	}
}

// A caption an author writes for a photograph is theirs. It was carried only as
// alternative text, so a region beside the picture stared back at the room while
// the words that belonged in it were invisible.
func TestAnImageCaptionFillsAFreeRegion(t *testing.T) {
	manifest := testManifest()
	images := func(reference string) (ContentImage, bool) {
		return ContentImage{AssetID: "asset-1", Name: reference}, true
	}
	options := CompileOptions{Language: "ko", ResolveImage: images}

	// A layout with a second region free: the caption goes there, as a caption
	// rather than as a point.
	result := Compile(ParseSource("# 제목\n@two\n::image 로고 | 지난달 문을 연 신규 매장입니다\n"), manifest, options)
	content := Decode(result.Slides[0].Content)
	caption := ""
	for slot, paragraphs := range content.Fields {
		if slot == pptx.SlotTitle || len(paragraphs) == 0 {
			continue
		}
		caption = paragraphs[0].Text
		if !paragraphs[0].Lead {
			t.Error("a caption is not one of the slide's points")
		}
	}
	if caption != "지난달 문을 연 신규 매장입니다" {
		t.Errorf("the caption is not on the slide: %+v", content.Fields)
	}

	// And it never writes over what the slide already says.
	withPoints := Compile(ParseSource("# 제목\n@two\n::image 로고 | 캡션\n- 첫 요점\n- 두 번째 요점\n"), manifest, options)
	said := Decode(withPoints.Slides[0].Content)
	points := 0
	for slot, paragraphs := range said.Fields {
		if slot == pptx.SlotTitle {
			continue
		}
		for _, paragraph := range paragraphs {
			if strings.Contains(paragraph.Text, "요점") {
				points++
			}
		}
	}
	if points != 2 {
		t.Errorf("the slide kept %d of its two points: %+v", points, said.Fields)
	}
}

// A number on a slide is asked about before anything else: where is it from.
// The language carries the answer beside the claim, and a round trip keeps it.
func TestASlideCitesWhereItsFiguresCameFrom(t *testing.T) {
	manifest := testManifest()
	source := "# 실적\n- 매출 1,240억 ^1\n- 이익률 9.8% ^2\n" +
		"!source 1 | 2026 시장 조사 보고서 | p.42\n!source 2 | 사내 결산 자료\n!notes 숫자는 출처와 함께 말합니다\n"
	parsed := ParseSource(source)
	if len(parsed.Warnings) != 0 {
		t.Fatalf("the source does not parse cleanly: %v", parsed.Warnings)
	}
	if len(parsed.Slides[0].Sources) != 2 {
		t.Fatalf("parsed %d sources", len(parsed.Slides[0].Sources))
	}
	first := parsed.Slides[0].Sources[0]
	if first.Marker != "1" || first.Title != "2026 시장 조사 보고서" || first.Locator != "p.42" {
		t.Errorf("the first source reads %+v", first)
	}
	if second := parsed.Slides[0].Sources[1]; second.Marker != "2" || second.Title != "사내 결산 자료" || second.Locator != "" {
		t.Errorf("a source with no locator reads %+v", second)
	}

	compiled := Compile(parsed, manifest, CompileOptions{Language: "ko"})
	content := Decode(compiled.Slides[0].Content)
	if len(content.Sources) != 2 {
		t.Fatalf("the slide stored %d sources", len(content.Sources))
	}
	written := Format(model.Presentation{Language: "ko", Slides: compiled.Slides}, manifest)
	for _, line := range []string{"!source 1 | 2026 시장 조사 보고서 | p.42", "!source 2 | 사내 결산 자료"} {
		if !strings.Contains(written, line) {
			t.Errorf("the round trip lost %q:\n%s", line, written)
		}
	}
}

// "!notes이 성장세는 부하로 이어집니다" is the directive and a sentence, written
// without the space between them — which is what happens when the next word
// begins with a Korean particle. Reading it as one token dropped the 이.
func TestADirectiveWithAWordStuckToItKeepsTheWord(t *testing.T) {
	parsed := ParseSource("# 시장 성장\n- 온라인 거래액 28.5% 증가\n" +
		"!notes 통계청 발표에 따릅니다.\n!notes이 성장세는 부하로 이어집니다.\n")
	if len(parsed.Warnings) != 0 {
		t.Fatalf("a stuck directive warned: %v", parsed.Warnings)
	}
	notes := parsed.Slides[0].Notes
	if !strings.Contains(notes, "이 성장세는 부하로 이어집니다") {
		t.Fatalf("the word was lost: %q", notes)
	}
	if !strings.Contains(notes, "통계청 발표에 따릅니다") {
		t.Fatalf("the first note was lost: %q", notes)
	}

	// A source written the same way keeps its first word too.
	cited := ParseSource("# 실적\n- 매출 1,240억\n!source통계청 | 2026 소비 동향 | 표 3\n")
	if len(cited.Slides[0].Sources) != 1 {
		t.Fatalf("the citation was not read: %+v", cited)
	}
	if got := cited.Slides[0].Sources[0]; got.Title != "통계청" {
		t.Fatalf("the source reads %+v", got)
	}

	// A word that could be part of a directive's own name is not split off: only
	// text that cannot belong to a directive name — Korean, a digit — is.
	latin := ParseSource("# 제목\n!notesomething 한 줄\n")
	if strings.Contains(latin.Slides[0].Notes, "omething") {
		t.Fatalf("a Latin word was cut in half: %q", latin.Slides[0].Notes)
	}
	digits := ParseSource("# 제목\n!notes2026년 계획을 말합니다\n")
	if !strings.Contains(digits.Slides[0].Notes, "2026년 계획을 말합니다") {
		t.Fatalf("a note beginning with a year lost it: %q", digits.Slides[0].Notes)
	}
}

// A comparison slide names both of its columns. On a template whose layout has
// one body region, both columns land in that region and the second heading sits
// in the middle of the list — where writing the source back used to drop it.
// Open the code view of such a deck, apply it, and the right-hand column lost
// its name and its points joined the left one.
func TestFormatKeepsAColumnHeadingFoundMidBody(t *testing.T) {
	manifest := testManifest()
	source := "# 투자 대비 리스크 분석\n@comparison\n> 투자 비용\n- 12억 원 투자 필요\n- 기술 부채 해소\n" +
		"> 잠재 손실\n- 거래 중단 매출 손실\n- 고객 이탈 장기 손실\n"
	first := Compile(ParseSource(source), manifest, CompileOptions{Language: "ko"})
	formatted := Format(model.Presentation{Slides: first.Slides}, manifest)
	if !strings.Contains(formatted, "> 잠재 손실") {
		t.Fatalf("the second column's heading was not written back:\n%s", formatted)
	}
	second := Compile(ParseSource(formatted), manifest, CompileOptions{Language: "ko"})
	before, after := Decode(first.Slides[0].Content), Decode(second.Slides[0].Content)
	if len(before.Fields[pptx.SlotBody]) != len(after.Fields[pptx.SlotBody]) {
		t.Fatalf("a round trip changed the body:\n%+v\n%+v", before.Fields[pptx.SlotBody], after.Fields[pptx.SlotBody])
	}
	for index := range before.Fields[pptx.SlotBody] {
		if before.Fields[pptx.SlotBody][index] != after.Fields[pptx.SlotBody][index] {
			t.Fatalf("line %d changed: %+v then %+v", index+1,
				before.Fields[pptx.SlotBody][index], after.Fields[pptx.SlotBody][index])
		}
	}
}

// A slide kept for the questions afterwards is part of the deck and part of the
// file; it is only not part of the talk. Saying so in the source is what makes
// it survive an export, a duplicate and a restore.
func TestASkippedSlideSurvivesARoundTrip(t *testing.T) {
	manifest := testManifest()
	source := "# 본론\n@content\n- 첫 줄\n\n# 부록: 상세 수치\n@content\n!skip\n- 물어보면 보여 줄 표\n"
	first := Compile(ParseSource(source), manifest, CompileOptions{Language: "ko"})
	if len(first.Slides) != 2 {
		t.Fatalf("compiled %d slides", len(first.Slides))
	}
	if Decode(first.Slides[0].Content).Skipped {
		t.Error("the slide that is part of the talk was marked skipped")
	}
	if !Decode(first.Slides[1].Content).Skipped {
		t.Fatal("the appendix slide was not marked skipped")
	}
	formatted := Format(model.Presentation{Slides: first.Slides}, manifest)
	if !strings.Contains(formatted, "!skip") {
		t.Fatalf("writing the deck back lost the mark:\n%s", formatted)
	}
	second := Compile(ParseSource(formatted), manifest, CompileOptions{Language: "ko"})
	if !Decode(second.Slides[1].Content).Skipped || Decode(second.Slides[0].Content).Skipped {
		t.Fatalf("a round trip moved or lost the mark:\n%s", formatted)
	}
	if len(second.Warnings) > 0 {
		t.Fatalf("the directive was not understood: %v", second.Warnings)
	}
}
