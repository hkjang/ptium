package export

import (
	"errors"
	"fmt"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pdf"
	"github.com/hkjang/ptium/server/internal/pptx"
)

// PDF puts the deck on paper: one page per slide, at the deck's own size.
//
// Each page is the drawing the workspace already makes of that slide —
// the same one the rail, the preview, the shared link and the presenting screen
// show — translated into PDF. Nothing here lays a slide out a second time,
// because a second layout is a second answer to where a line sits, and the two
// would drift.
//
// The type is not the deck's. A PowerPoint template names its typeface and does
// not carry it, and a PDF reader has no Korean face of its own, so the file is
// set in the one face the workspace ships. The exported pptx is where the
// deck's own design lives; this is where it can be read anywhere.
func PDF(presentation model.Presentation, options Options) ([]byte, error) {
	if len(presentation.Slides) == 0 {
		return nil, errors.New("the presentation does not contain any slide")
	}
	manifest := options.Manifest
	if manifest.Version != pptx.ManifestVersion || len(manifest.Layouts) == 0 {
		if len(options.TemplateData) == 0 {
			return nil, errors.New("the presentation template is unavailable")
		}
		_, analyzed, err := pptx.AnalyzeBytes(options.TemplateData)
		if err != nil {
			return nil, fmt.Errorf("analyze presentation template: %w", err)
		}
		manifest = analyzed
	}
	font, err := pdf.BuiltinFont()
	if err != nil {
		return nil, fmt.Errorf("read the built-in font: %w", err)
	}
	widthEMU, heightEMU := manifest.SlideWidth, manifest.SlideHeight
	if widthEMU <= 0 || heightEMU <= 0 {
		widthEMU, heightEMU = 12192000, 6858000
	}
	// A point is 12700 EMU. Asking the drawing for exactly as many pixels as the
	// page has points puts the two in the same units, so a font size in the
	// drawing is the same number of points on paper.
	width := float64(widthEMU) / 12700
	height := float64(heightEMU) / 12700
	document := pdf.New(width, height, presentation.Title, font)
	built := deck.BuildWithImages(presentation, manifest, options.Author, options.Images)
	for index, slide := range built.Slides {
		if slide.Skipped {
			// A slide kept out of the talk is kept out of the handout: the paper
			// is what the room is given, and PowerPoint hides it too.
			continue
		}
		layout, ok := manifest.Layout(slide.LayoutID)
		if !ok {
			if layout, ok = manifest.LayoutForRole(pptx.RoleContent); !ok {
				continue
			}
		}
		slide.Number = index + 1
		drawing := pptx.PreviewSVG(manifest, layout, slide, pptx.PreviewOptions{
			Width: int(width), Media: options.Media, Language: presentation.Language})
		page := document.AddPage()
		if err := pdf.DrawSVG(page, drawing); err != nil {
			return nil, fmt.Errorf("draw slide %d: %w", index+1, err)
		}
	}
	if document.Pages() == 0 {
		return nil, errors.New("every slide in the deck is skipped")
	}
	return document.Bytes(), nil
}
