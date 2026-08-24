// Package export turns a stored presentation into a downloadable file.
package export

import (
	"errors"
	"fmt"
	"strings"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

// Options carries everything needed to render a deck into its template.
type Options struct {
	// TemplateData is the stored .pptx/.potx package the deck was designed for.
	TemplateData []byte
	// Manifest is the analysis cached alongside the template. A stale or empty
	// manifest is recomputed from the package rather than trusted.
	Manifest pptx.Manifest
	// Author is written into the document properties.
	Author string
	// Images resolves the pictures a deck places. Without it a deck's images are
	// left out rather than failing the export.
	Images deck.ImageSource
	// Media resolves the template's own artwork — the pictures a design paints
	// on its layouts — for the renderings that draw the design themselves.
	Media pptx.MediaResolver
}

// PPTX renders a presentation into a PowerPoint file that reuses the
// customer's own master, layouts, theme, fonts and imagery. Only the slides
// are generated; nothing about the design is recreated from scratch.
func PPTX(presentation model.Presentation, options Options) ([]byte, error) {
	if len(presentation.Slides) == 0 {
		return nil, errors.New("the presentation does not contain any slide")
	}
	if len(options.TemplateData) == 0 {
		return nil, errors.New("the presentation template is unavailable")
	}
	pkg, err := pptx.Open(options.TemplateData)
	if err != nil {
		return nil, fmt.Errorf("read presentation template: %w", err)
	}
	manifest := options.Manifest
	if manifest.Version != pptx.ManifestVersion || len(manifest.Layouts) == 0 {
		manifest, err = pptx.Analyze(pkg)
		if err != nil {
			return nil, fmt.Errorf("analyze presentation template: %w", err)
		}
	}
	// The file carries its own source so that importing it gives the deck back
	// rather than a reading of its drawing. That only holds while the source
	// still describes the slides; when the canvas has moved on, the deck as it
	// stands is written instead of the text it was last saved from.
	if !deck.MatchesSlides(presentation.Source, presentation, manifest) {
		presentation.Source = deck.Format(presentation, manifest)
	}
	return pptx.Render(pkg, manifest, deck.BuildWithImages(presentation, manifest, options.Author, options.Images))
}

// PreviewSVG renders one slide as scalable vector graphics so the workspace
// can show the real template design without a PowerPoint engine.
func PreviewSVG(presentation model.Presentation, manifest pptx.Manifest, position, width int,
	media pptx.MediaResolver, images deck.ImageSource) (string, error) {
	return PreviewSlideSVG(presentation, manifest, position, pptx.PreviewOptions{Width: width, Media: media}, images)
}

// PreviewSlideSVG is PreviewSVG with the renderer's own options, so a caller can
// ask for the slide without the template's background — which is how the canvas
// lifts one region off the page to drag it.
func PreviewSlideSVG(presentation model.Presentation, manifest pptx.Manifest, position int,
	options pptx.PreviewOptions, images deck.ImageSource) (string, error) {
	layout, slide, err := PreviewSlide(presentation, manifest, position, images)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(options.Language) == "" {
		options.Language = presentation.Language
	}
	return pptx.PreviewSVG(manifest, layout, slide, options), nil
}

// PreviewSlide binds one stored slide to its layout, which is what both the
// renderer and the editor need before they can say anything about it.
func PreviewSlide(presentation model.Presentation, manifest pptx.Manifest, position int,
	images deck.ImageSource) (pptx.Layout, pptx.Slide, error) {
	if position < 1 || position > len(presentation.Slides) {
		return pptx.Layout{}, pptx.Slide{}, fmt.Errorf("slide %d does not exist", position)
	}
	if len(manifest.Layouts) == 0 {
		return pptx.Layout{}, pptx.Slide{}, errors.New("the presentation template is unavailable")
	}
	built := deck.BuildWithImages(presentation, manifest, "", images)
	slide := built.Slides[position-1]
	layout, ok := manifest.Layout(slide.LayoutID)
	if !ok {
		layout = manifest.Layouts[0]
	}
	return layout, slide, nil
}
