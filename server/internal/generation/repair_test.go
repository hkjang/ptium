package generation

import (
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

// A label drawn past the side of its own box is the author's words, so the
// answer is a shorter line rather than a smaller drawing — which only works if
// the finding reaches the pass that asks for one.
func TestAComponentLabelTooWideIsSentBackForARewrite(t *testing.T) {
	template, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatalf("builtin template: %v", err)
	}
	_, manifest, err := pptx.AnalyzeBytes(template)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	long := "지식관리 시스템 교체에 따른 이관 일정과 담당 조직의 준비 상태"
	source := "# 준비 상태\n@two\n> 왼쪽\n- 한 줄\n> 오른쪽\n::meter 진행\n- " + long + " | 72%\n::\n"
	compiled := deck.Compile(deck.ParseSource(source), manifest, deck.CompileOptions{Language: "ko"})
	presentation := model.Presentation{Language: "ko", Title: "준비 상태", Slides: compiled.Slides}

	reported := false
	for _, finding := range pptx.InspectDeck(manifest, deck.Build(presentation, manifest, "")) {
		if strings.Contains(finding.Detail, "wider than") {
			reported = true
			if !repairable(finding) {
				t.Fatalf("the finding is not worth another draft: %s", finding.String())
			}
		}
	}
	if !reported {
		t.Skip("this template draws the label inside its box; nothing to repair")
	}
	defects := slidesByDefect(manifest, presentation)
	if len(defects) == 0 {
		t.Fatal("the slide was not queued for a rewrite")
	}
	if defects[0].action != ReviseFit {
		t.Fatalf("the slide is queued for %q, wanted a shorter line", defects[0].action)
	}
	said := false
	for _, detail := range defects[0].details {
		said = said || strings.Contains(detail, "wider than")
	}
	if !said {
		t.Fatalf("the rewrite is not told what is too wide: %q", defects[0].details)
	}
}

// A rewrite that deleted something always measures better. The slide that found
// this had two columns and the rewrite came back with one, the other's heading
// simply gone — and the defect count went down, so it was kept.
func TestARewriteThatDropsAHeadingIsRefused(t *testing.T) {
	template, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatalf("builtin template: %v", err)
	}
	_, manifest, err := pptx.AnalyzeBytes(template)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	compile := func(source string) model.Slide {
		result := deck.Compile(deck.ParseSource(source), manifest, deck.CompileOptions{Language: "ko"})
		if len(result.Slides) != 1 {
			t.Fatalf("compiling %q made %d slides", source, len(result.Slides))
		}
		return result.Slides[0]
	}
	before := compile("# 조직 준비 상태\n@two\n> 준비된 것\n- 핵심 인력 교육 완료\n> 남은 것\n- 변경 관리 프로세스 수립\n")
	dropped := compile("# 조직 준비 상태\n@two\n> 준비된 것\n- 핵심 인력 교육 완료\n- 변경 관리 프로세스 수립\n")
	if lost := structureLost(before, dropped); lost == "" {
		t.Fatal("a rewrite that dropped a column heading was accepted")
	}
	// Shorter words with the same shape are what a rewrite is for.
	shorter := compile("# 조직 준비 상태\n@two\n> 준비된 것\n- 인력 교육 완료\n> 남은 것\n- 변경 관리 수립\n")
	if lost := structureLost(before, shorter); lost != "" {
		t.Fatalf("a rewrite that only shortened its lines was refused: %s", lost)
	}
	// So is merging two lines into one, as long as the slide keeps its shape.
	merged := compile("# 조직 준비 상태\n@two\n> 준비된 것\n- 인력 교육과 변경 관리 완료\n> 남은 것\n- 이관 일정 확정\n")
	if lost := structureLost(before, merged); lost != "" {
		t.Fatalf("a rewrite that merged two lines was refused: %s", lost)
	}
}

// A component that holds more than it draws is worth another draft: the entries
// past the limit are on no slide, which is the same kind of loss as text that
// does not fit.
//
// Against the live model this could not be fixed by asking. Told in two places
// that a process takes at most five stages, and given a brief naming eight, it
// wrote eight — first as a timeline, then as steps. So the deck is measured
// after it is written and the slide is sent back with what was measured.
func TestAComponentDrawingLessThanItHoldsIsSentBackForARewrite(t *testing.T) {
	template, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatalf("builtin template: %v", err)
	}
	_, manifest, err := pptx.AnalyzeBytes(template)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	source := "# 이행 순서\n@content\n::steps 순서\n- 요구사항 확정 | 현업 프로세스 매핑\n" +
		"- 데이터 정제 | 레거시 품질 검증\n- 파일럿 구축 | 핵심 모듈 시험\n- 1차 이관 | 비핵심 부서\n" +
		"- 2차 이관 | 핵심 부서\n- 병행 운영 | 양 시스템 가동\n- 안정화 | 성능 최적화\n- 최종 전환 | 레거시 종료\n::\n"
	compiled := deck.Compile(deck.ParseSource(source), manifest, deck.CompileOptions{Language: "ko"})
	presentation := model.Presentation{Language: "ko", Title: "이행 순서", Slides: compiled.Slides}

	defects := slidesByDefect(manifest, presentation)
	if len(defects) == 0 {
		t.Fatal("a slide holding eight stages of a five-stage component was not queued for a rewrite")
	}
	if defects[0].action != ReviseShorten {
		t.Fatalf("the slide is queued for %q, wanted a shorter one", defects[0].action)
	}
	said := false
	for _, detail := range defects[0].details {
		said = said || strings.Contains(detail, "of its 8 entries")
	}
	if !said {
		t.Fatalf("the rewrite is not told what is left out: %q", defects[0].details)
	}

	// A rewrite that keeps five of them measures better and is kept; one that
	// still holds eight is not an improvement.
	shorter := deck.Compile(deck.ParseSource("# 이행 순서\n@content\n::steps 순서\n- 요구사항 확정 | 현업 매핑\n"+
		"- 데이터 정제 | 품질 검증\n- 이관 | 부서별 전환\n- 병행 운영 | 양 시스템\n- 최종 전환 | 레거시 종료\n::\n"),
		manifest, deck.CompileOptions{Language: "ko"}).Slides
	if got := defectsOnSlide(manifest, presentation, 1, shorter[0]); got != 0 {
		t.Errorf("the shorter slide still measures %d defects", got)
	}
	if got := defectsOnSlide(manifest, presentation, 1, presentation.Slides[0]); got == 0 {
		t.Error("the slide that holds eight measures clean, so no rewrite could be judged better")
	}
}
