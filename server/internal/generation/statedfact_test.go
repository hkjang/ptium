package generation

import (
	"strings"
	"testing"
)

// A brief that states its facts is a good brief. It was being used twice.
//
// "사내 PostgreSQL 전환 타당성 검토 결과를 경영진에게 보고해줘. 현재 오라클
// 라이선스가 연 4억이고, 전환 대상은 리포팅 DB 12개, 목표는 2026년 3분기 완료야."
// — the figures were read out of those clauses, and then the same clauses became
// slide headings, cut wherever the reader had stopped: "현재 오라클 라이선스가 연",
// "전환 대상은 리포팅 DB", "목표는 2026년 3분기 완료야". Three of eight slides in
// the deck were headed with a fragment of the brief, and the deck's own
// measurement scored it 99 and said nothing.
func TestAStatedFactIsNotASlideHeading(t *testing.T) {
	outline := outlinePrompt(
		"사내 PostgreSQL 전환 타당성 검토 결과를 경영진에게 보고해줘. 현재 오라클 라이선스가 연 4억이고, "+
			"전환 대상은 리포팅 DB 12개, 목표는 2026년 3분기 완료야.", "", koreanCopy)
	if len(outline.Topics) != 1 || outline.Topics[0].Name != "전환 타당성 검토 결과" {
		names := []string{}
		for _, topic := range outline.Topics {
			names = append(names, topic.Name)
		}
		t.Errorf("the brief's facts became headings: %q", names)
	}
	// The facts are not lost — they are what the deck draws.
	if outline.Period != "2026년 3분기" {
		t.Errorf("the period was dropped: %q", outline.Period)
	}
	values := []string{}
	for _, figure := range outline.Figures {
		values = append(values, figure.Value)
	}
	for _, want := range []string{"4억", "12개"} {
		if !containsValue(values, want) {
			t.Errorf("the brief said %q and the outline does not carry it: %q", want, values)
		}
	}
}

// Both halves of the rule are needed. A marked subject on its own would take a
// question with it, and a figure on its own would take a subject that happens
// to count something.
func TestASubjectIsNotMistakenForAStatedFact(t *testing.T) {
	for _, want := range []struct{ brief, topic string }{
		{"위험은 무엇인가와 대응 방안을 8장으로 정리해줘", "위험은 무엇인가"},
		{"클라우드 비용 최적화 3개년 로드맵을 10장으로 작성해줘", "클라우드 비용 최적화 3개년 로드맵"},
		{"채용 계획, 교육 계획, 평가 제도 개편을 12장으로 정리해줘", "채용 계획"},
	} {
		outline := outlinePrompt(want.brief, "", koreanCopy)
		found := false
		for _, topic := range outline.Topics {
			if strings.Contains(topic.Name, want.topic) {
				found = true
			}
		}
		if !found {
			names := []string{}
			for _, topic := range outline.Topics {
				names = append(names, topic.Name)
			}
			t.Errorf("brief %q lost the topic %q: %q", want.brief, want.topic, names)
		}
	}
}

func containsValue(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
