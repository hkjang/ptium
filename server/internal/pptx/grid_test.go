package pptx

import "testing"

// The shipped checklist labels a cell "진행" and explains it as "진행 중". Joined
// with a space the legend read "진행 진행 중".
func TestALegendDoesNotStutter(t *testing.T) {
	cases := map[[2]string]string{
		{"진행", "진행 중"}:  "진행 중",
		{"완료", "확인됨"}:   "완료 확인됨",
		{"R", "실행"}:     "R 실행",
		{"미착수", "미착수"}:  "미착수",
		{"조치 필요", "조치"}: "조치 필요",
	}
	for input, want := range cases {
		if got := legendEntry(input[0], input[1]); got != want {
			t.Fatalf("legendEntry(%q, %q) = %q, wanted %q", input[0], input[1], got, want)
		}
	}
}
