package generation

import (
	"strings"
	"testing"
)

// A brief written as prose says what it is about before it says anything about
// it, and a topic cut out of the middle of that sentence is not a heading.
//
// Six prose briefs measured here produced covers reading "…크게 줄여야",
// "데이터 거버넌스 체계를 세우", "너무 오래 걸린다는 이야기가" — each one a
// fragment sliced out of a sentence. Where a topic comes out cut like that, the
// phrase the brief's own first sentence marks is what the deck is about.
//
// Two of the six are repaired by this and the rest are not: a brief whose first
// sentence marks nothing usable is still cut, and the measurement now reports
// that rather than the product hiding it.
func TestACutTopicIsRepairedFromTheBriefsOwnSentence(t *testing.T) {
	for _, want := range []struct{ brief, topic string }{
		{"우리 회사는 내년에 클라우드 비용을 크게 줄여야 합니다. 지금 구조로는 계속 늘어납니다.", "클라우드 비용"},
		{"데이터 거버넌스 체계를 세우려고 합니다. 어디서부터 시작해야 할지 정리가 필요합니다.", "데이터 거버넌스 체계"},
	} {
		outline := outlinePrompt(want.brief, "", koreanCopy)
		found := false
		for _, topic := range outline.Topics {
			if strings.Contains(topic.Name, want.topic) {
				found = true
			}
			if cutPhrase(topic.Name) {
				t.Errorf("brief %q still yields a cut topic: %q", want.brief[:20], topic.Name)
			}
		}
		if !found {
			names := []string{}
			for _, topic := range outline.Topics {
				names = append(names, topic.Name)
			}
			t.Errorf("brief %q does not name %q: %q", want.brief[:20], want.topic, names)
		}
	}
}

// A brief that asks for something keeps every topic it asked for. The repair is
// only for a topic that came out cut, and a request-style brief does not.
func TestRepairingProseLeavesARequestAlone(t *testing.T) {
	for _, want := range []struct{ brief, topic string }{
		{"위험은 무엇인가와 대응 방안을 8장으로 정리해줘", "위험은 무엇인가"},
		{"채용 계획, 교육 계획, 평가 제도 개편을 12장으로 정리해줘", "평가 제도 개편"},
		{"보고 체계 개선 방안을 8장으로 정리해줘", "보고 체계 개선 방안"},
		{"클라우드 비용 최적화 3개년 로드맵을 경영진 대상 10장으로 작성해줘", "클라우드 비용 최적화 3개년 로드맵"},
		{"AI 엔지니어링팀 1분기 업무 성과와 2분기 계획을 10장으로 정리해줘", "2분기 계획"},
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
			t.Errorf("brief %q lost %q: %q", want.brief[:24], want.topic, names)
		}
	}
}

// And the cover follows. The cover takes the brief's subject whole when it fits,
// which for prose is most of a sentence cut mid-verb — "우리 회사는 내년에
// 클라우드 비용을 크게 줄여야" was the first thing a room would have read.
func TestACutSubjectDoesNotBecomeTheCover(t *testing.T) {
	for _, want := range []struct{ brief, cover string }{
		{"우리 회사는 내년에 클라우드 비용을 크게 줄여야 합니다. 지금 구조로는 계속 늘어납니다.", "클라우드 비용"},
		{"데이터 거버넌스 체계를 세우려고 합니다. 어디서부터 시작해야 할지 정리가 필요합니다.", "데이터 거버넌스 체계"},
	} {
		outline := outlinePrompt(want.brief, "", koreanCopy)
		cover := outline.deckTitle("", want.brief, " · ")
		if cutPhrase(cover) {
			t.Errorf("the cover still stops mid-sentence: %q", cover)
		}
		if !strings.Contains(cover, want.cover) {
			t.Errorf("the cover %q does not name %q", cover, want.cover)
		}
	}
	// A brief that asks for something keeps the cover it asked for.
	for _, want := range []struct{ brief, cover string }{
		{"사내 PostgreSQL 전환 타당성 검토 결과를 경영진에게 12장으로 보고해줘", "사내 PostgreSQL 전환 타당성 검토 결과"},
		{"위험은 무엇인가와 대응 방안을 8장으로 정리해줘", "위험은 무엇인가"},
		{"클라우드 비용 최적화 3개년 로드맵을 경영진 대상 10장으로 작성해줘", "클라우드 비용 최적화 3개년 로드맵"},
	} {
		outline := outlinePrompt(want.brief, "", koreanCopy)
		if cover := outline.deckTitle("", want.brief, " · "); !strings.Contains(cover, want.cover) {
			t.Errorf("brief %q lost its cover: %q", want.brief[:22], cover)
		}
	}
}
