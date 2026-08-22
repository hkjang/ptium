package golden

import (
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

// The inspector's findings are read back over the fixtures, because a finding
// that fires on a good slide costs more than one that never fires: the score
// drops on work that is right, and people stop reading the score.
func TestTheFixturesAreOnlyToldWhatIsTrueOfThem(t *testing.T) {
	for _, fixture := range fixtures {
		template, err := pptx.BuiltinTemplate(fixture.design)
		if err != nil {
			t.Fatalf("%s: %v", fixture.name, err)
		}
		_, manifest, err := pptx.AnalyzeBytes(template)
		if err != nil {
			t.Fatalf("%s: %v", fixture.name, err)
		}
		compiled := deck.Compile(deck.ParseSource(fixture.source), manifest, deck.CompileOptions{
			Language: "ko",
			ResolveImage: func(reference string) (deck.ContentImage, bool) {
				return deck.ContentImage{AssetID: "asset-" + reference, Name: reference}, true
			},
		})
		built := deck.BuildWithImages(model.Presentation{Title: fixture.title, Language: "ko",
			Slides: compiled.Slides}, manifest, "Ptium", fixedPicture)
		for _, finding := range pptx.InspectDeck(manifest, built) {
			// Nothing in these decks overflows, collides, or leaves the page. The
			// advisories they do carry — no notes, no sources — are true of them.
			if !finding.Advisory {
				t.Errorf("%s: %s", fixture.name, finding.String())
			}
			if strings.Contains(finding.Detail, "overlap") {
				t.Errorf("%s: %s", fixture.name, finding.String())
			}
		}
		if score := pptx.ScoreDeck(pptx.InspectDeck(manifest, built), len(built.Slides)); score.Total < 85 {
			t.Errorf("%s scores %d, which is lower than anything in it deserves", fixture.name, score.Total)
		}
	}
}
