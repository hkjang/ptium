package store

import "strings"

import "testing"

// The wildcards belong to the query, not to the reader.
//
// "%"+text+"%" hands what somebody typed straight to LIKE: searching for "%"
// listed the whole library, "50%" matched "500만원" as readily as "50%", and a
// name with an underscore in it — metadata_demo, AI_Coding_ROI — could not be
// searched for exactly.
func TestWhatSomebodyTypesIsNotAWildcard(t *testing.T) {
	cases := map[string]string{
		"클라우드":           `%클라우드%`,
		"50%":            `%50\%%`,
		"metadata_demo":  `%metadata\_demo%`,
		`back\slash`:     `%back\\slash%`,
		"  공백 ":          `%공백%`,
		"%":              `%\%%`,
		"_":              `%\_%`,
		"AI_Coding_ROI%": `%AI\_Coding\_ROI\%%`,
	}
	for typed, want := range cases {
		if got := likePattern(typed); got != want {
			t.Errorf("likePattern(%q) = %q, want %q", typed, got, want)
		}
	}
	// And every LIKE built from one has to say how to read the escape.
	if !strings.Contains(likeEscape, `ESCAPE '\'`) {
		t.Errorf("likeEscape = %q", likeEscape)
	}
}
