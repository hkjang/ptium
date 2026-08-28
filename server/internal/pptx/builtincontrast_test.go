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
