package pptx

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The screens have to know which colour means "nobody chose one", because the
// drawing ignores exactly that value. The server says so in its settings
// answer; the web carries the same colour as a fallback for an older server.
// Two copies of one colour drift, and the day they drift the personalisation
// screen goes back to promising a colour that will never be drawn.
func TestTheWebCarriesTheSameSeededColour(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile("../../../web/src/branding/accent.ts")
	if err != nil {
		t.Skipf("web source not readable here: %v", err)
	}
	found := regexp.MustCompile(`seededAccent\s*=\s*'(#[0-9A-Fa-f]{6})'`).FindSubmatch(source)
	if found == nil {
		t.Fatal("the web no longer names a seeded accent colour")
	}
	if got := string(found[1]); !strings.EqualFold(got, SeededAccent) {
		t.Errorf("the web says the seeded colour is %s, the drawing ignores %s", got, SeededAccent)
	}
}
