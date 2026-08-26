package deck

import (
	"testing"

	"github.com/hkjang/ptium/server/internal/pptx"
)

// The last slide of a deck is drawn as a closing page because decks end that
// way, not because a dozen points happen to be on it. A closing layout holds a
// title and an ask; an imported deck whose last slide was twelve points of what
// the product could go on to do came out as twelve lines in room for three.
func TestALastSlideFullOfPointsIsNotAClosingPage(t *testing.T) {
	manifest := testManifest()
	full := Compile(ParseSource("# 표지\n> 오늘 결정할 것\n\n# 현황\n- 요점\n\n# 기대효과\n"+
		"- 확장성\n- 서비스 범위 확장\n- 다양한 산업군 적용\n- 멀티채널 지원\n- 기능 확장\n- 다국어 지원\n"),
		manifest, CompileOptions{Language: "ko"})
	if last := full.Outline[len(full.Outline)-1]; last.Role == pptx.RoleClosing {
		t.Errorf("a slide of six points was drawn as a closing page: %+v", last)
	}
	// A deck that ends the way decks end still does.
	ending := Compile(ParseSource("# 표지\n> 오늘 결정할 것\n\n# 현황\n- 요점\n\n# 다음 단계\n- 오늘 요청하는 결정 한 가지\n"),
		manifest, CompileOptions{Language: "ko"})
	if last := ending.Outline[len(ending.Outline)-1]; last.Role != pptx.RoleClosing {
		t.Errorf("a deck that ends on one ask was drawn as %q", last.Role)
	}
	// And an author who asks for a closing page gets one.
	asked := Compile(ParseSource("# 표지\n> 오늘\n\n# 현황\n- 요점\n\n# 맺음\n@closing\n"+
		"- 하나\n- 둘\n- 셋\n- 넷\n- 다섯\n"), manifest, CompileOptions{Language: "ko"})
	if last := asked.Outline[len(asked.Outline)-1]; last.Role != pptx.RoleClosing {
		t.Errorf("@closing was not obeyed: %q", last.Role)
	}
}
