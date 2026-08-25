package deck

import (
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/pptx"
)

// A deck someone already has comes across as its argument: the points at the
// depth they were written, the notes, the kind of each slide, and the tables —
// with what could not be carried said out loud rather than dropped in silence.
func TestSourceFromImportKeepsTheArgument(t *testing.T) {
	imported := pptx.ImportedDeck{Title: "지난 분기 보고", Slides: []pptx.ImportedSlide{
		{Title: "2025년 4분기 영업 실적", Lead: "영업기획팀", Role: pptx.RoleTitle, Notes: "결론을 먼저 말합니다."},
		{Title: "실적 요약", Bullets: []pptx.ImportedLine{
			{Text: "매출 1,240억"}, {Text: "신규 채널이 절반", Level: 1}}},
		{Title: "채널별 매출", Tables: [][][]string{{{"채널", "3분기"}, {"직영", "420억"}}},
			Pictures: []pptx.ImportedPicture{{Name: "image1.png", Data: []byte("png"), Area: 400}}, OtherCharts: 1},
	}}
	source, warnings := SourceFromImport(imported)

	for _, line := range []string{
		"# 2025년 4분기 영업 실적", "@cover", "> 영업기획팀", "!notes 결론을 먼저 말합니다.",
		"- 매출 1,240억", "  - 신규 채널이 절반", "::table", "- 채널 | 3분기", "- 직영 | 420억",
	} {
		if !strings.Contains(source, line) {
			t.Fatalf("the import lost %q:\n%s", line, source)
		}
	}
	// The source it produces is the source it can read back.
	parsed := ParseSource(source)
	if len(parsed.Warnings) != 0 {
		t.Fatalf("the imported source does not parse cleanly: %v", parsed.Warnings)
	}
	if len(parsed.Slides) != 3 {
		t.Fatalf("parsed %d slides", len(parsed.Slides))
	}
	said := map[string]bool{}
	for _, warning := range warnings {
		switch {
		case strings.Contains(warning, "그림"):
			said["pictures"] = true
		case strings.Contains(warning, "표"):
			said["tables"] = true
		case strings.Contains(warning, "차트"):
			said["charts"] = true
		}
	}
	for _, kind := range []string{"pictures", "tables", "charts"} {
		if !said[kind] {
			t.Fatalf("the import said nothing about %s: %v", kind, warnings)
		}
	}
}

// A chart Ptium wrote comes back as its numbers: export, import, and the
// figures are still there to be drawn again.
func TestImportedChartsKeepTheirNumbers(t *testing.T) {
	template, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatalf("builtin template: %v", err)
	}
	built := pptx.Deck{Language: "ko", Title: "차트 왕복", Slides: []pptx.Slide{
		{LayoutID: "title", Fields: map[string][]pptx.Paragraph{pptx.SlotTitle: {{Text: "차트 왕복"}}}},
		{LayoutID: "content", Fields: map[string][]pptx.Paragraph{pptx.SlotTitle: {{Text: "분기 매출"}}},
			Blocks: map[string]pptx.Block{pptx.SlotBody: {Kind: pptx.BlockColumns, Items: []pptx.Item{
				{Label: "1분기", Value: "1,180"}, {Label: "2분기", Value: "1,240"}, {Label: "3분기", Value: "1,390"}}}}},
		{LayoutID: "content", Fields: map[string][]pptx.Paragraph{pptx.SlotTitle: {{Text: "추이"}}},
			Blocks: map[string]pptx.Block{pptx.SlotBody: {Kind: pptx.BlockLine, Labels: []string{"1월", "2월", "3월"},
				Series: []pptx.Series{{Name: "매출", Points: []float64{110, 130, 150}}}}}},
	}}
	data, err := pptx.RenderBytes(template, built)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	pkg, err := pptx.Open(data)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	source, _ := SourceFromImport(pptx.ReadDeck(pkg))
	for _, line := range []string{"::columns", "- 1분기 | 1180", "- 3분기 | 1390", "::line", "- 기간 | 1월, 2월, 3월", "- 매출 | 110, 130, 150"} {
		if !strings.Contains(source, line) {
			t.Fatalf("the round trip lost %q:\n%s", line, source)
		}
	}
	parsed := ParseSource(source)
	if len(parsed.Warnings) != 0 {
		t.Fatalf("the imported source does not parse cleanly: %v", parsed.Warnings)
	}
}

// Not every plot has a component that says the same thing. Several series of
// columns become a table rather than a trend, and a pie becomes the share bar
// Ptium draws a division of one whole as.
func TestImportedPlotsBecomeWhatTheyMean(t *testing.T) {
	source, _ := SourceFromImport(pptx.ImportedDeck{Slides: []pptx.ImportedSlide{
		{Title: "분기별", Charts: []pptx.ImportedChart{{Kind: pptx.BlockColumns,
			Categories: []string{"1분기", "2분기"}, Series: []pptx.Series{
				{Name: "매출", Points: []float64{120, 150}}, {Name: "비용", Points: []float64{90, 95}}}}}},
		{Title: "점유", Charts: []pptx.ImportedChart{{Kind: pptx.BlockShare,
			Categories: []string{"자사", "타사"}, Series: []pptx.Series{{Points: []float64{60, 40}}}}}},
	}})
	for _, line := range []string{"::table", "- 구분 | 1분기 | 2분기", "- 매출 | 120 | 150",
		"::share", "- 자사 | 60", "- 타사 | 40"} {
		if !strings.Contains(source, line) {
			t.Fatalf("the import lost %q:\n%s", line, source)
		}
	}
	if parsed := ParseSource(source); len(parsed.Warnings) != 0 {
		t.Fatalf("the imported source does not parse cleanly: %v", parsed.Warnings)
	}
}

// And the source it becomes says so, because that is where the deck keeps it.
func TestImportedHiddenSlidesAreSkipped(t *testing.T) {
	source, _ := SourceFromImport(pptx.ImportedDeck{Slides: []pptx.ImportedSlide{
		{Title: "보이는 장", Bullets: []pptx.ImportedLine{{Text: "요점"}}},
		{Title: "숨긴 장", Bullets: []pptx.ImportedLine{{Text: "아직 공유하지 않을 내용"}}, Hidden: true},
	}})
	if strings.Count(source, "!skip") != 1 {
		t.Fatalf("the hidden slide is not marked skipped:\n%s", source)
	}
	shown, hidden, _ := strings.Cut(source, "# 숨긴 장")
	if strings.Contains(shown, "!skip") {
		t.Errorf("the slide that is part of the show was marked skipped:\n%s", shown)
	}
	if !strings.Contains(hidden, "!skip") {
		t.Errorf("the hidden slide lost its mark:\n%s", hidden)
	}
}

// And it is written where the deck keeps citations.
func TestImportedCitationsAreWrittenAsSources(t *testing.T) {
	source, _ := SourceFromImport(pptx.ImportedDeck{Slides: []pptx.ImportedSlide{
		{Title: "근거", Bullets: []pptx.ImportedLine{{Text: "요점"}}, Sources: []string{"내부 자료 2026"}},
	}})
	if !strings.Contains(source, "!source 내부 자료 2026") {
		t.Errorf("the citation is not written as one:\n%s", source)
	}
	if strings.Contains(source, "- 출처") {
		t.Errorf("the citation is still a point:\n%s", source)
	}
}
