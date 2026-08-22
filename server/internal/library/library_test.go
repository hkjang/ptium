package library

import (
	"strings"
	"testing"
)

func entries() []Entry {
	return []Entry{
		{ID: "a", Name: "회사 소개", Source: "# 회사 소개\n@content\n> 2003년 설립\n- 임직원 1,240명\n- 매출 8,200억\n"},
		{ID: "b", Name: "보안 아키텍처", Aliases: []string{"보안 구조"},
			Source: "# 보안 아키텍처\n::steps 3계층\n- 경계 | WAF와 IPS\n- 내부 | 제로 트러스트\n- 데이터 | 암호화\n::\n"},
	}
}

// A company has slides that must not vary. When a deck writes one of them, the
// registered slide takes its place — the words someone already agreed.
func TestARegisteredSlideTakesItsPlace(t *testing.T) {
	source := "# 2026 사업 계획\n@cover\n> 경영진 보고\n\n# 회사 소개\n- 우리 회사는 좋은 회사입니다\n\n" +
		"# 올해 목표\n- 매출 1조\n\n# 보안 구조\n- 방화벽이 있습니다\n"
	written, used := Substitute(source, entries())
	if len(used) != 2 {
		t.Fatalf("used %d registered slides: %+v", len(used), used)
	}
	for _, wanted := range []string{"- 임직원 1,240명", "::steps 3계층", "- 데이터 | 암호화"} {
		if !strings.Contains(written, wanted) {
			t.Errorf("the deck did not take %q:\n%s", wanted, written)
		}
	}
	for _, unwanted := range []string{"우리 회사는 좋은 회사입니다", "방화벽이 있습니다"} {
		if strings.Contains(written, unwanted) {
			t.Errorf("the deck kept the written-from-scratch %q", unwanted)
		}
	}
	// What nobody registered is left exactly as the deck wrote it.
	if !strings.Contains(written, "# 올해 목표\n- 매출 1조") {
		t.Errorf("an unregistered slide was changed:\n%s", written)
	}
	// And the cover is never replaced: it carries the deck's own title.
	if !strings.HasPrefix(written, "# 2026 사업 계획\n@cover") {
		t.Errorf("the cover was replaced:\n%s", written)
	}
}

// A wrong substitution puts something else in front of the room, so matching is
// strict: a title has to name the slide, not merely mention a word from it.
func TestOnlyAClearMatchSubstitutes(t *testing.T) {
	for _, title := range []string{"계획", "회사 소개를 곁들인 2026년 상반기 사업 계획과 조직 개편 보고", "소개"} {
		if _, ok := Match(title, entries()); ok {
			t.Errorf("%q matched a registered slide", title)
		}
	}
	for _, title := range []string{"회사 소개", "회사소개", "회사 소개 (2026)", "보안 구조"} {
		if _, ok := Match(title, entries()); !ok {
			t.Errorf("%q did not match the slide it names", title)
		}
	}
}

// A registered slide is one page. A deck that argues a subject across two
// slides matches both to the same entry, and putting it in twice would give the
// company introduction twice.
func TestARegisteredSlideIsUsedOncePerDeck(t *testing.T) {
	source := "# 덱 제목\n@cover\n\n# 회사 소개\n- 생성기가 쓴 줄\n\n# 회사 소개 — 비용과 효과\n- 또 생성기가 쓴 줄\n\n# 다른 주제\n- 한 줄\n"
	entries := []Entry{{ID: "s1", Name: "회사 소개", Source: "# 회사 소개\n- 임직원 1,240명\n"}}
	written, used := Substitute(source, entries)
	if len(used) != 1 {
		t.Fatalf("the entry was used %d times: %+v", len(used), used)
	}
	if strings.Count(written, "임직원 1,240명") != 1 {
		t.Fatalf("the registered slide went in more than once:\n%s", written)
	}
	// And the slide it did not take keeps what generation wrote for it.
	if !strings.Contains(written, "또 생성기가 쓴 줄") {
		t.Fatalf("the second slide lost its own text:\n%s", written)
	}
}
