package generation

import (
	"strings"
	"testing"
)

// The imperative a brief ends with must not become the deck.
//
// "…보고해줘" is one word to the person writing it. The subject reader took the
// verb and left the auxiliary, so a deck came back titled "사내 PostgreSQL 전환
// 타당성 검토 결과를 줘" — and because every heading is built from the subject,
// those three syllables sat on all nine slides. Five of nine ordinary briefs
// measured this way carried an instruction into the title: "…를 줘", "…을 써줘",
// "…을 주세요", "…을 써 주세요", "…을 경영진 대상".
//
// The product's own measurement had been flagging them all along — "the heading
// stops in the middle of what it was saying" — which is worth saying plainly:
// the deck was shipped with a defect the deck itself had already found.
func TestABriefsImperativeDoesNotBecomeTheDeck(t *testing.T) {
	for _, want := range []struct{ brief, subject string }{
		{"사내 PostgreSQL 전환 타당성 검토 결과를 경영진에게 12장으로 보고해줘", "사내 PostgreSQL 전환 타당성 검토 결과"},
		{"보안 점검 결과를 이사회에 보고해줘", "보안 점검 결과"},
		{"데이터 품질 개선 방안을 팀장에게 보고해 주세요", "데이터 품질 개선 방안"},
		{"전사 데이터 거버넌스 체계 수립 방안을 임원 보고용 10장으로 써줘", "전사 데이터 거버넌스 체계 수립 방안"},
		{"신규 채용 계획을 5장으로 써 주세요", "신규 채용 계획"},
		{"클라우드 비용 최적화 3개년 로드맵을 경영진 대상 10장으로 작성해줘", "클라우드 비용 최적화 3개년 로드맵"},
		{"채용 절차 개선안을 고객 대상으로 정리해 주세요", "채용 절차 개선안"},
	} {
		outline := outlinePrompt(want.brief, "", koreanCopy)
		if outline.Subject != want.subject {
			t.Errorf("brief %q\n  subject %q\n  want    %q", want.brief, outline.Subject, want.subject)
		}
		for _, topic := range outline.Topics {
			if endsWithRequest(topic.Name) {
				t.Errorf("brief %q produced a topic that ends in an instruction: %q", want.brief, topic.Name)
			}
		}
	}
}

// A subject that is genuinely about reporting keeps its own words: "대상" is an
// ordinary noun, and a deck about a reporting process is about reporting.
func TestAnOrdinarySubjectIsNotMistakenForAnInstruction(t *testing.T) {
	for _, want := range []struct{ brief, keep string }{
		{"분석 대상 데이터셋 선정 기준을 정리해줘", "분석 대상"},
		{"월간 보고서 자동화 방안을 정리해줘", "보고서"},
	} {
		outline := outlinePrompt(want.brief, "", koreanCopy)
		if !strings.Contains(outline.Subject, want.keep) {
			t.Errorf("brief %q lost %q: subject is %q", want.brief, want.keep, outline.Subject)
		}
	}
}

func endsWithRequest(name string) bool {
	for _, tail := range []string{"줘", "주세요", "주시고", "주십시오", "대상"} {
		if strings.HasSuffix(name, tail) {
			return true
		}
	}
	return false
}
