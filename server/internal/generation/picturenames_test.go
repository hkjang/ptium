package generation

import "testing"

// The list handed to a model is names and nothing else — it cannot look at the
// pictures — so a name that says nothing is an invitation to guess. A live
// model put "해커톤 최종발표 · image22.png" on an English warehouse deck and
// again on a Japanese quarterly one.
func TestOnlyPicturesWhoseNamesSaySomethingAreOffered(t *testing.T) {
	said := map[string]bool{
		"현장 자동화":                             true,
		"조직도":                                true,
		"AMR 로봇 20대":                         true,
		"2026 로드맵":                           true,
		"해커톤_최종발표(해커톤_TeamKCB) (1) · image22.png": false,
		"보고서 · image7.png":                    false,
		"IMG_4821.jpg":                       false,
		"screenshot 2026-08-26.png":          false,
		"image.png":                          false,
		"사진.png":                             false,
		"무제":                                 false,
		"logo-716734.png":                    true, // a logo is a thing somebody named
		"":                                   false,
	}
	for name, wanted := range said {
		if got := saysWhatItIs(name); got != wanted {
			t.Errorf("saysWhatItIs(%q) = %v, want %v", name, got, wanted)
		}
	}
}
