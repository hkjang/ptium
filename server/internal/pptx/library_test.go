package pptx

import "testing"

func TestDesignLibraryIsBalanced(t *testing.T) {
	designs := BuiltinDesigns()
	if len(designs) != 50 {
		t.Fatalf("the library ships %d designs, want 50", len(designs))
	}
	families, palettes, keys := map[string]int{}, map[string]int{}, map[string]bool{}
	covers, bodies, motifs := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, design := range designs {
		families[design.Family.Key]++
		palettes[design.Palette.Key]++
		covers[design.Family.Cover] = true
		bodies[design.Family.Body] = true
		if design.Family.Motif != "" {
			motifs[design.Family.Motif] = true
		}
		if keys[design.Key] {
			t.Fatalf("duplicate design key %q", design.Key)
		}
		keys[design.Key] = true
	}
	if len(families) != len(layoutFamilies) {
		t.Fatalf("every layout family should be shipped, got %d of %d: %v", len(families), len(layoutFamilies), families)
	}
	for palette, count := range palettes {
		if count != 5 {
			t.Fatalf("palette %s appears %d times, want 5", palette, count)
		}
	}
	// Every metaphor ships, and every palette has one: a deck about growth and a
	// deck about risk should not be able to open with the same picture.
	if len(motifs) != 6 {
		t.Fatalf("the library ships %d metaphors, want 6: %v", len(motifs), motifs)
	}
	// A library is varied when its slides are composed differently, not when the
	// same composition is recoloured. Every cover and body composition ships.
	if len(covers) != 7 {
		t.Fatalf("the library ships %d cover compositions, want 7: %v", len(covers), covers)
	}
	if len(bodies) != 3 {
		t.Fatalf("the library ships %d body compositions, want 3: %v", len(bodies), bodies)
	}
	// Every design the library shipped before must still resolve, or a deck built
	// on one would silently change design.
	for _, shipped := range []string{
		"slate-classic", "slate-panel", "slate-minimal", "azure-classic", "azure-rail", "azure-centered",
		"crimson-classic", "crimson-editorial", "crimson-panel", "coral-rail", "coral-centered", "coral-editorial",
		"ivory-editorial", "ivory-centered", "ivory-classic", "sand-editorial", "sand-minimal", "sand-panel",
		"midnight-panel", "midnight-rail", "midnight-minimal", "graphite-minimal", "graphite-classic", "graphite-rail",
		"forest-panel", "forest-centered", "forest-minimal", "plum-rail", "plum-centered", "plum-editorial",
	} {
		if !keys[shipped] {
			t.Fatalf("design %q disappeared from the library", shipped)
		}
	}
	// Legacy theme names must still resolve to a shipped design.
	for _, legacy := range []string{"aurora", "modern", "paper", "mint", "graphite", "", "nonsense"} {
		if design := LookupBuiltinDesign(legacy); design.Key == "" {
			t.Fatalf("%q did not resolve to a design", legacy)
		}
	}
}
