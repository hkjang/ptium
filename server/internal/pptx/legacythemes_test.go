package pptx

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The screens have to show which design a stored theme value means. A value an
// older version wrote — "aurora", "paper", "mint" — is not a design key, and a
// screen that cannot read it shows the wrong design to everybody who has been
// here a while. The web carries the same table; two copies drift.
func TestTheWebReadsTheSameLegacyThemeNames(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile("../../../web/src/branding/designs.ts")
	if err != nil {
		t.Skipf("web source not readable here: %v", err)
	}
	table := regexp.MustCompile(`legacyThemeAliases[^{]*\{([^}]*)\}`).FindSubmatch(source)
	if table == nil {
		t.Fatal("the web no longer carries a legacy theme table")
	}
	written := map[string]string{}
	for _, pair := range regexp.MustCompile(`(\w+)\s*:\s*'([a-z-]+)'`).FindAllStringSubmatch(string(table[1]), -1) {
		written[pair[1]] = pair[2]
	}
	for name, key := range legacyDesignAliases {
		if got := written[name]; got != key {
			t.Errorf("the web reads %q as %q; the server answers it with %q", name, got, key)
		}
	}
	for name, key := range written {
		if _, known := legacyDesignAliases[name]; !known {
			t.Errorf("the web reads %q as %q; the server has never heard of it", name, key)
		}
	}
	// Every design the table names must still be one this product ships.
	for name, key := range legacyDesignAliases {
		if design := LookupBuiltinDesign(key); !strings.EqualFold(design.Key, key) {
			t.Errorf("%q points at %q, which is no longer a shipped design", name, key)
		}
	}
}

// What a stored theme value selects. The screens answer this question too — a
// picker has to show which design the value in the profile means — so the rules
// are pinned here and mirrored in web/src/branding/designs.test.ts.
func TestWhatAStoredThemeSelects(t *testing.T) {
	t.Parallel()
	first := BuiltinDesigns()[0].Key
	for stored, want := range map[string]string{
		"slate-classic":         "slate-classic", // a design key is itself
		"aurora":                "plum-rail",     // a name an older version stored
		"modern":                "slate-classic",
		"graphite":              "graphite-minimal", // a bare family: its first design
		"crimson":               "crimson-classic",
		"":                      first, // nothing at all falls back to the library's first
		"a-design-nobody-ships": first,
	} {
		if got := LookupBuiltinDesign(stored).Key; got != want {
			t.Errorf("%q selects %q, expected %q", stored, got, want)
		}
	}
	// The rank the templates listing answers with is that same order.
	if BuiltinDesignRank(first) != 1 {
		t.Errorf("the library's first design ranks %d", BuiltinDesignRank(first))
	}
	if BuiltinDesignRank("a-design-nobody-ships") != 0 {
		t.Error("a design this product does not ship was given a rank")
	}
}
