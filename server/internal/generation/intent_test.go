package generation

import "testing"

func TestParseIntentReadsTheRequestedSlideCount(t *testing.T) {
	cases := map[string]int{
		"3장만 만들어줘":                         3,
		"클라우드 전환 로드맵 3장으로 요약해줘":            3,
		"슬라이드 5개로 정리해 주세요":                 5,
		"10페이지 분량의 사업계획서":                  10,
		"신규 모바일 서비스의 투자 유치를 위한 12장짜리 피치덱":  12,
		"5~7장 정도로":                         7,
		"7-9 slides on the migration plan": 9,
		"make a 4 slide deck":              4,
		"three slides about onboarding":    3,
		"두 장으로 압축해줘":                       2,
		"열두 장 정도의 발표자료":                    12,
		"슬라이드는 6장 이내로":                     6,
		"장수는 4개":                           4,
		"slide count: 8":                   8,
		"deck of 9 for the board":          9,
		// Nothing about length.
		"클라우드 전환 로드맵과 투자 타당성": 0,
		"": 0,
		// Units that are not slides must not be read as a length.
		"3분기 실적 요약":   0,
		"10년 성장 전략":   0,
		"2025년 3월 실적": 0,
		"제품 3개 비교":    0,
		"3개월 로드맵":     0,
		// 장 as a chapter heading, not a count.
		"제3장 위험 관리를 설명해줘": 0,
		// A number too large to be a deck length is ignored rather than being
		// silently truncated into a plausible one.
		"120장으로 만들어줘": 0,
		"1000 slides": 0,
	}
	for prompt, want := range cases {
		if got := ParseIntent(prompt).SlideCount; got != want {
			t.Fatalf("ParseIntent(%q).SlideCount = %d, want %d", prompt, got, want)
		}
	}
}

func TestParseIntentReadsLanguageAndAudience(t *testing.T) {
	intent := ParseIntent("임원 보고용으로 영어로 5장 만들어줘")
	if intent.SlideCount != 5 || intent.Language != "en" || intent.Audience != "임원" {
		t.Fatalf("intent = %+v", intent)
	}
	if plain := ParseIntent("팀 협업 도구 소개"); plain.Language != "" || plain.Audience != "" {
		t.Fatalf("a prompt without instructions must stay empty: %+v", plain)
	}
	if investors := ParseIntent("투자 유치를 위한 피치덱"); investors.Audience != "투자자" {
		t.Fatalf("audience = %q", investors.Audience)
	}
}

func TestApplySlideCountPrefersTheExplicitRequest(t *testing.T) {
	intent := Intent{SlideCount: 3}
	if got := intent.ApplySlideCount(7, 8, 50); got != 7 {
		t.Fatalf("an explicit request must win: %d", got)
	}
	if got := intent.ApplySlideCount(0, 8, 50); got != 3 {
		t.Fatalf("the prompt must beat the default: %d", got)
	}
	if got := (Intent{}).ApplySlideCount(0, 8, 50); got != 8 {
		t.Fatalf("the default must apply when nothing else does: %d", got)
	}
	if got := intent.ApplySlideCount(0, 8, 2); got != 2 {
		t.Fatalf("the deployment maximum must cap the result: %d", got)
	}
	if got := (Intent{}).ApplySlideCount(0, 0, 50); got != 1 {
		t.Fatalf("a nonsensical default must still produce a slide: %d", got)
	}
}

// The length the request asked for, before this deployment's cap.
//
// The cap was applied at the front door with nothing said: a request for ten
// slides where five are allowed came back with a five-slide deck and no reason.
// The caller can only say so if it can tell what was asked for.
func TestWhatTheRequestAskedForBeforeTheCap(t *testing.T) {
	spoken := Intent{SlideCount: 12}
	if got := spoken.SlideCountAsked(0, 3); got != 12 {
		t.Errorf("a brief asking for 12 was read as %d", got)
	}
	if got := spoken.SlideCountAsked(7, 3); got != 7 {
		t.Errorf("an explicit 7 over a brief's 12 was read as %d", got)
	}
	if got := (Intent{}).SlideCountAsked(0, 3); got != 3 {
		t.Errorf("a request that said nothing was read as %d, want the default 3", got)
	}
}

// And the answer is still capped, whatever was asked.
func TestTheCapStillHolds(t *testing.T) {
	spoken := Intent{SlideCount: 12}
	if got := spoken.ApplySlideCount(0, 3, 5); got != 5 {
		t.Errorf("a brief asking for 12 where 5 are allowed made %d", got)
	}
	if got := spoken.ApplySlideCount(0, 3, 50); got != 12 {
		t.Errorf("a brief asking for 12 where 50 are allowed made %d", got)
	}
	// Nothing was cut, so nothing has to be said.
	if asked, made := (Intent{}).SlideCountAsked(4, 3), (Intent{}).ApplySlideCount(4, 3, 5); asked != made {
		t.Errorf("a request inside the cap asked %d and made %d", asked, made)
	}
}
