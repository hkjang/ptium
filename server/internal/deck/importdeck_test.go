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
			Pictures: []pptx.ImportedPicture{{Name: "image1.png", Data: []byte("png"), Area: 400}}, Charts: 1},
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
