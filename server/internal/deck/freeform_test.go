package deck

import (
	"encoding/json"
	"testing"

	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

func TestValidateFreeformElementsRejectsUnsafeDocuments(t *testing.T) {
	valid := FreeformElement{ID: "text-1", Kind: "text", X: 10, Y: 12, Width: 30, Height: 15, Text: "hello"}
	if err := ValidateFreeformElements([]FreeformElement{valid}); err != nil {
		t.Fatalf("valid element rejected: %v", err)
	}
	for name, elements := range map[string][]FreeformElement{
		"duplicate id":       {valid, valid},
		"unknown kind":       {{ID: "x", Kind: "video", Width: 10, Height: 10}},
		"missing asset":      {{ID: "x", Kind: "image", Width: 10, Height: 10}},
		"outside bounds":     {{ID: "x", Kind: "shape", X: 500, Width: 10, Height: 10}},
		"uneven table":       {{ID: "table", Kind: "table", Width: 40, Height: 20, Cells: [][]string{{"a", "b"}, {"c"}}}},
		"unknown line style": {{ID: "line", Kind: "line", Width: 20, Height: 2, Dash: "scribble"}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateFreeformElements(elements); err == nil {
				t.Fatal("invalid elements were accepted")
			}
		})
	}
}

func TestBuildConvertsFreeformPercentagesAndResolvesPictures(t *testing.T) {
	manifest := testManifest()
	content := Content{Type: ContentType, LayoutID: "content", Elements: []FreeformElement{
		{ID: "shape-1", Kind: "shape", Shape: "ellipse", X: 10, Y: 20, Width: 25, Height: 30, Fill: "725BD6", ZIndex: 2},
		{ID: "image-1", Kind: "image", AssetID: "asset-1", X: 5, Y: 5, Width: 20, Height: 20, ZIndex: 1},
		{ID: "table-1", Kind: "table", X: 20, Y: 55, Width: 50, Height: 25, Cells: [][]string{{"항목", "상태"}, {"A", "완료"}}, HeaderRows: 1, ZIndex: 3},
	}}
	raw, _ := json.Marshal(content)
	presentation := model.Presentation{Title: "Freeform", Slides: []model.Slide{{Title: "Slide", Layout: "content", LayoutID: "content", Content: raw}}}
	built := BuildWithImages(presentation, manifest, "", func(assetID string) (pptx.Picture, bool) {
		if assetID != "asset-1" {
			return pptx.Picture{}, false
		}
		return pptx.Picture{Data: []byte("image"), ContentType: "image/png", Width: 100, Height: 100}, true
	})
	if len(built.Slides) != 1 || len(built.Slides[0].Elements) != 3 {
		t.Fatalf("freeform elements were not built: %+v", built.Slides)
	}
	// Lower z-index images are emitted first even when stored later.
	image, shape := built.Slides[0].Elements[0], built.Slides[0].Elements[1]
	if image.ID != "image-1" || image.Picture == nil || shape.ID != "shape-1" {
		t.Fatalf("z-order or image resolution was lost: %+v", built.Slides[0].Elements)
	}
	if shape.Frame.X != manifest.SlideWidth/10 || shape.Frame.Y != manifest.SlideHeight/5 || shape.Frame.Width != manifest.SlideWidth/4 {
		t.Fatalf("percentage geometry was converted incorrectly: %+v", shape.Frame)
	}
	table := built.Slides[0].Elements[2]
	if table.Kind != "table" || len(table.Cells) != 2 || table.Cells[1][1] != "완료" || table.HeaderRows != 1 {
		t.Fatalf("table cells were not carried into the renderer: %+v", table)
	}
}
