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
