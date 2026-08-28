package pptx

import "testing"

// The cover's one-line summary is prose somebody reads. Mixed toward the
// surface by a fixed amount it landed at 2.2:1 on thirty-two of the designs
// this product ships — every one of them below even the 3:1 that large text is
// allowed, and one at 1.55:1 — while every ink the design itself derives has
// kept a contrast floor all along.
//
// This product's own contrast measurement never saw it: it inspects the
// regions Ptium composes, and a template's placeholders are the template's
// business. These templates are Ptium's.
func TestEveryShippedCoverSubtitleIsLegible(t *testing.T) {
	for _, palette := range builtinPalettes {
		ink := fadeInk(palette.Ink, palette.Surface, 0.42, 4.5)
		if ratio := contrastRatio(ink, palette.Surface); ratio < 4.5 {
			t.Errorf("%s: a cover subtitle of %s on %s is %.2f:1, below 4.5:1",
				palette.Key, ink, palette.Surface, ratio)
		}
		// And it is still a quieter grey than the heading beside it, which is
		// what a subtitle is for: legible, not loud.
		if contrastRatio(ink, palette.Surface) > contrastRatio(palette.Ink, palette.Surface) {
			t.Errorf("%s: the subtitle is stronger than the heading", palette.Key)
		}
	}
}

// The inks a design derives are drawn on the panels it raises out of the
// surface — a KPI tile, a card, a banded row — and they were measured only
// against the surface. On a dark design a KPI label stood at 9.1:1 against the
// surface and 3.6:1 on the tile it is actually written on.
//
// They also kept the floor for large text while being set at 12pt, which is
// not large text: across the fifty designs this ships, a six-slide deck drew
// 440 words below the contrast their size calls for.
func TestTheInksADesignDerivesAreLegibleWhereTheyAreDrawn(t *testing.T) {
	for _, palette := range builtinPalettes {
		surface, ink := palette.Surface, palette.Ink
		raised := mixColor(surface, ink, ifElse(relativeLuminance(surface) < 0.4, 0.10, 0.045))
		secondary := fadeInkAgainst(ink, surface, raised, 0.28, 4.5)
		muted := fadeInkAgainst(ink, surface, raised, 0.48, 4.5)
		for _, one := range []struct {
			name string
			ink  string
		}{{"secondary", secondary}, {"muted", muted}} {
			for _, behind := range []struct {
				what  string
				color string
			}{{"the surface", surface}, {"a raised panel", raised}} {
				if ratio := contrastRatio(one.ink, behind.color); ratio < 4.5 {
					t.Errorf("%s: %s ink %s on %s (%s) is %.2f:1, below 4.5:1",
						palette.Key, one.name, one.ink, behind.color, behind.what, ratio)
				}
			}
		}
		// Quieter than the ink it stands beside, which is what these registers
		// are for: legible, not loud.
		if contrastRatio(muted, surface) >= contrastRatio(ink, surface) {
			t.Errorf("%s: the muted ink is no quieter than the primary one", palette.Key)
		}
	}
}
