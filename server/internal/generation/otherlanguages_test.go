package generation

import (
	"strings"
	"testing"
)

// Japanese and Chinese are offered on the same screen as Korean and English,
// and neither had any instruction reading at all: "新しい採用計画を8枚でまとめて
// ください" became, word for word, the title of the deck it asked for — the
// count, the verb and the politeness with it. English was half-done: the verb
// was taken and the rest of the request left standing, so "Write me a 8 slide
// deck about reducing cloud spend" produced a deck called "Me a deck about
// reducing cloud spend".
func TestARequestInAnyOfferedLanguageIsNotTheSubject(t *testing.T) {
	for _, want := range []struct{ language, brief, subject string }{
		{"en", "Write me a 8 slide deck about reducing cloud spend", "reducing cloud spend"},
		{"en", "Please create a 10 slide presentation on our data warehouse migration", "our data warehouse migration"},
		{"en", "Make a short deck about hiring plans for the leadership team", "hiring plans"},
		{"ja", "新しい採用計画を8枚でまとめてください", "新しい採用計画"},
		{"ja", "クラウド費用の最適化について10枚で作成してください", "クラウド費用の最適化"},
		{"ja", "データ基盤移行のリスクを経営層向けに12枚でお願いします", "データ基盤移行のリスク"},
		{"zh", "请用10页整理一下我们的云成本优化方案", "我们的云成本优化方案"},
		{"zh", "把数据仓库迁移的风险做成12页的汇报", "数据仓库迁移的风险"},
	} {
		outline := outlinePrompt(want.brief, "", copyFor(want.language))
		if outline.Subject != want.subject {
			t.Errorf("[%s] %q\n  subject %q\n  want    %q", want.language, want.brief, outline.Subject, want.subject)
		}
	}
}

// The room is stripped only when it is named. "Any word before 向け" reaches
// backwards through the subject: it turned a deck about the risks of a data
// platform migration into a deck about "データ".
func TestNamingTheRoomDoesNotSwallowTheSubject(t *testing.T) {
	outline := outlinePrompt("データ基盤移行のリスクを経営層向けに12枚でお願いします", "", japaneseCopy)
	if !strings.Contains(outline.Subject, "リスク") {
		t.Errorf("the subject lost what it was about: %q", outline.Subject)
	}
	// A word that merely ends in 向け is not a room.
	kept := outlinePrompt("配送の方向性と改善案を10枚でまとめてください", "", japaneseCopy)
	if !strings.Contains(kept.Subject, "方向性") {
		t.Errorf("an ordinary word was read as an audience: %q", kept.Subject)
	}
}

// The other half of reading a request: a sentence that merely opens with one of
// those words is a subject. Every line here came back mangled from the first
// attempt at this — "Make or buy decision for our data platform" became a deck
// about "or buy decision", "Generate reports faster" became "s faster", 请示
// lost its 请, and a Chinese rule reached into Japanese and made 整理整頓 into
// 整頓. Stripping too much is the same defect as stripping too little; it is
// just harder to notice, because the title still reads like a title.
func TestASubjectThatOpensLikeARequestIsStillASubject(t *testing.T) {
	for _, want := range []struct{ language, brief string }{
		{"en", "Make or buy decision for our data platform"},
		{"en", "Write-ahead logging in PostgreSQL explained"},
		{"en", "Create better onboarding for new hires"},
		{"en", "Generate reports faster with the new pipeline"},
		{"en", "Build vs buy analysis for the CRM"},
		{"zh", "请示流程改进方案"},
		{"zh", "整理需求的标准流程"},
		{"ja", "資料作成プロセスの改善"},
		{"ja", "整理整頓の社内ルール"},
		{"ja", "配送の方向性と改善案"},
		{"ko", "보고 체계 개선 방안"},
		{"ko", "분석 대상 데이터셋 선정 기준"},
		{"ko", "월간 보고 자동화 방안"},
	} {
		outline := outlinePrompt(want.brief, "", copyFor(want.language))
		if outline.Subject != want.brief {
			t.Errorf("[%s] a subject was read as an instruction\n  brief   %q\n  subject %q",
				want.language, want.brief, outline.Subject)
		}
	}
}

func copyFor(language string) languageCopy {
	switch language {
	case "ja":
		return japaneseCopy
	case "zh":
		return chineseCopy
	case "en":
		return englishCopy
	}
	return koreanCopy
}
