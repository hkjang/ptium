package pptx

import "testing"

func TestDesignLibraryIsBalanced(t *testing.T) {
	designs := BuiltinDesigns()
	if len(designs) != 30 {
		t.Fatalf("the library ships %d designs, want 30", len(designs))
	}
	families, palettes, keys := map[string]int{}, map[string]int{}, map[string]bool{}
	for _, design := range designs {
		families[design.Family.Key]++
		palettes[design.Palette.Key]++
		if keys[design.Key] {
			t.Fatalf("duplicate design key %q", design.Key)
		}
		keys[design.Key] = true
	}
	if len(families) != 6 {
		t.Fatalf("expected six layout families, got %d: %v", len(families), families)
	}
	for family, count := range families {
		if count != 5 {
			t.Fatalf("family %s appears %d times, want 5: %v", family, count, families)
		}
	}
	for palette, count := range palettes {
		if count != 3 {
			t.Fatalf("palette %s appears %d times, want 3", palette, count)
		}
	}
	// Legacy theme names must still resolve to a shipped design.
	for _, legacy := range []string{"aurora", "modern", "paper", "mint", "graphite", "", "nonsense"} {
		if design := LookupBuiltinDesign(legacy); design.Key == "" {
			t.Fatalf("%q did not resolve to a design", legacy)
		}
	}
}
