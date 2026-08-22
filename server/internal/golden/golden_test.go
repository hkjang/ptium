package golden

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

var update = flag.Bool("update", false, "rewrite the recorded outlines from what the renderer produces now")

// The fixtures are decks a person would actually give a room: a cover with a
// lead, an argument in points, a table, a plotted trend, a division of one
// whole, and a claim with the source it came from. Between them they reach
// every kind of part a rendered deck has.
var fixtures = []struct {
	name   string
	design string
	title  string
	source string
}{
	{
		name:   "strategy",
		design: "plum-rail",
		title:  "2026 성장 전략",
		source: "# 2026 성장 전략\n@cover\n> 국내 시장 재진입 로드맵\n!notes 오늘의 목적을 한 문장으로 밝힙니다.\n\n" +
			"# 핵심 진단\n- 기존 채널의 성장률이 3분기 연속 둔화\n  - 이탈 고객의 62%가 온보딩 단계에서 발생\n" +
			"- 신규 세그먼트에서만 두 자릿수 성장 유지\n!source 통계청 | 2026 소비 동향 | 표 3\n\n" +
			"# 연간 비용\n::table 연간 비용\n- 항목 | 2026 | 2027\n- 인건비 | 4.2억 | 3.4억\n- 라이선스 | 1.1억 | 1.4억\n::\n\n" +
			"# 월별 처리량\n::line 월별 처리량\n- 월 | 1월, 2월, 3월, 4월\n- 전환 전 | 120, 118, 121, 119\n- 전환 후 | 120, 132, 148, 165\n::\n",
	},
	{
		name:   "review",
		design: "",
		title:  "분기 리뷰",
		source: "# 분기 리뷰\n@cover\n> 영업기획팀\n\n" +
			"# 한눈에\n@two\n::kpi 핵심 지표\n- 매출 | 1,240억\n- 신규 고객 | 312곳\n::\n- 직영 채널이 성장을 이끌었습니다\n\n" +
			"# 이행 계획\n::steps\n- 준비 | 범위 확정\n- 이행 | 이관\n- 안정화 | 점검\n::\n",
	},
}

// A rendered deck is compared whole against one recorded earlier: every part,
// every element, every attribute. Most of what this catches is not a wrong
// number but a wrong file — a part that stopped being declared, a relationship
// that stopped resolving, a layout that stopped being inherited. PowerPoint
// notices those; a measurement does not.
func TestRenderedPackagesMatchTheirRecordedOutline(t *testing.T) {
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			outline, err := Outline(render(t, fixture.design, fixture.title, fixture.source))
			if err != nil {
				t.Fatalf("outline: %v", err)
			}
			path := filepath.Join("testdata", fixture.name+".outline.txt")
			if *update {
				if err := os.WriteFile(path, []byte(outline), 0o644); err != nil {
					t.Fatalf("write %s: %v", path, err)
				}
				t.Logf("recorded %s (%d lines)", path, strings.Count(outline, "\n"))
				return
			}
			recorded, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v (run: go test ./internal/golden -update)", path, err)
			}
			if diff := firstDifference(string(recorded), outline); diff != "" {
				t.Fatalf("the rendered file no longer matches %s:\n%s\n\n"+
					"If the change was the point, re-record it with: go test ./internal/golden -update", path, diff)
			}
		})
	}
}

// The same input renders the same file twice. Anything that varies between two
// runs — a map walked in whatever order, an id counted from a clock — would
// make the comparison above flap, so it is worth saying separately.
func TestRenderingTwiceProducesTheSameFile(t *testing.T) {
	fixture := fixtures[0]
	first, err := Outline(render(t, fixture.design, fixture.title, fixture.source))
	if err != nil {
		t.Fatalf("outline: %v", err)
	}
	second, err := Outline(render(t, fixture.design, fixture.title, fixture.source))
	if err != nil {
		t.Fatalf("outline: %v", err)
	}
	if diff := firstDifference(first, second); diff != "" {
		t.Fatalf("two renders of one deck differ:\n%s", diff)
	}
}

func render(t *testing.T, design, title, source string) []byte {
	t.Helper()
	template, err := pptx.BuiltinTemplate(design)
	if err != nil {
		t.Fatalf("builtin template %q: %v", design, err)
	}
	pkg, manifest, err := openTemplate(template)
	if err != nil {
		t.Fatalf("analyze template: %v", err)
	}
	result := deck.Compile(deck.ParseSource(source), manifest, deck.CompileOptions{Language: "ko"})
	if len(result.Slides) == 0 {
		t.Fatalf("the fixture compiled to no slides: %v", result.Warnings)
	}
	presentation := model.Presentation{Title: title, Language: "ko", Slides: result.Slides}
	data, err := pptx.Render(pkg, manifest, deck.Build(presentation, manifest, "Ptium"))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return data
}

func openTemplate(template []byte) (*pptx.Package, pptx.Manifest, error) {
	pkg, manifest, err := pptx.AnalyzeBytes(template)
	return pkg, manifest, err
}

// firstDifference reports the first line that differs with the lines around it,
// because a whole-file diff of two thousand lines says less than one place does.
func firstDifference(recorded, produced string) string {
	if recorded == produced {
		return ""
	}
	was, now := strings.Split(recorded, "\n"), strings.Split(produced, "\n")
	for index := 0; index < len(was) || index < len(now); index++ {
		before, after := line(was, index), line(now, index)
		if before == after {
			continue
		}
		var out strings.Builder
		for back := index - 3; back < index; back++ {
			if back >= 0 {
				out.WriteString("  " + line(was, back) + "\n")
			}
		}
		out.WriteString("- " + before + "\n+ " + after + "\n")
		for ahead := index + 1; ahead <= index+3; ahead++ {
			if ahead < len(was) || ahead < len(now) {
				out.WriteString("  was: " + line(was, ahead) + "\n  now: " + line(now, ahead) + "\n")
			}
		}
		out.WriteString("(line " + itoa(index+1) + " of " + itoa(len(was)) + " recorded, " + itoa(len(now)) + " produced)")
		return out.String()
	}
	return ""
}

func line(lines []string, index int) string {
	if index < 0 || index >= len(lines) {
		return "«end of file»"
	}
	return lines[index]
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := ""
	for value > 0 {
		digits = string(rune('0'+value%10)) + digits
		value /= 10
	}
	return digits
}
