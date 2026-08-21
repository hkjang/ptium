package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/export"
	"github.com/hkjang/ptium/server/internal/generation"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

// canvasRegion is one region of a slide as the canvas needs it: where it draws,
// what it holds, and how it is styled — in percentages of the slide, which is
// the only unit a browser and a PPTX can both be told without conversion.
type canvasRegion struct {
	Slot        string             `json:"slot"`
	Kind        string             `json:"kind"`
	Frame       deck.SlotFrame     `json:"frame"`
	Layout      deck.SlotFrame     `json:"layout"`
	Moved       bool               `json:"moved"`
	Style       *pptx.Style        `json:"style,omitempty"`
	Text        string             `json:"text,omitempty"`
	Paragraphs  []pptx.Paragraph   `json:"paragraphs,omitempty"`
	Block       *pptx.Block        `json:"block,omitempty"`
	Image       *deck.ContentImage `json:"image,omitempty"`
	FontSize    float64            `json:"fontSize,omitempty"`
	Bold        bool               `json:"bold,omitempty"`
	Italic      bool               `json:"italic,omitempty"`
	Align       string             `json:"align,omitempty"`
	Color       string             `json:"color,omitempty"`
	Font        string             `json:"font,omitempty"`
	Name        string             `json:"name,omitempty"`
	Prompt      string             `json:"prompt,omitempty"`
	AcceptsText bool               `json:"acceptsText"`
	SpannedBy   string             `json:"spannedBy,omitempty"`
}

// slideRegions describes one slide's template-bound content as editable objects.
//
// Without this the canvas can only draw on top of a generated slide, because the
// generated part is a picture to it. With it, the same click that selects a text
// box the author added selects the title the model wrote.
func (s *Server) slideRegions(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	presentation, err := s.store.GetPresentation(request.Context(), request.PathValue("id"), user.ID, false)
	if err != nil {
		s.handleStoreError(writer, request, err, "presentation_read_failed")
		return
	}
	position, ok := slidePosition(writer, request, len(presentation.Slides))
	if !ok {
		return
	}
	_, manifest, err := s.presentationTemplate(request.Context(), presentation)
	if err != nil {
		s.handleStoreError(writer, request, err, "presentation_template_unavailable")
		return
	}
	layout, slide, err := export.PreviewSlide(presentation, manifest, position, s.imageSource(request, user.ID))
	if err != nil {
		writeError(writer, request, http.StatusNotFound, "not_found", err.Error(), nil)
		return
	}
	content := deck.Decode(presentation.Slides[position-1].Content)
	width, height := manifest.SlideWidth, manifest.SlideHeight
	if width <= 0 || height <= 0 {
		width, height = 12192000, 6858000
	}
	percent := func(frame pptx.Frame) deck.SlotFrame {
		return deck.SlotFrame{
			X:      float64(frame.X) / float64(width) * 100,
			Y:      float64(frame.Y) / float64(height) * 100,
			Width:  float64(frame.Width) / float64(width) * 100,
			Height: float64(frame.Height) / float64(height) * 100,
		}
	}
	regions := make([]canvasRegion, 0, len(layout.Placeholders))
	for _, region := range pptx.SlideRegions(layout, slide) {
		converted := canvasRegion{
			Slot: region.Slot, Kind: region.Kind, Frame: percent(region.Frame), Layout: percent(region.Layout),
			Moved: region.Moved, Paragraphs: region.Paragraphs, Block: region.Block,
			Align: region.Align, Italic: region.Italic,
			FontSize: float64(region.FontSize) / 100, Bold: region.Bold, Color: region.Color, Font: region.Font,
			Name: region.Name, Prompt: region.Prompt, AcceptsText: region.Accepts, SpannedBy: region.Spanned,
		}
		if region.Kind == pptx.RegionText {
			converted.Text = region.Text()
		}
		if style, ok := content.Styles[region.Slot]; ok && !style.Empty() {
			copied := style
			converted.Style = &copied
		}
		if placed, ok := content.Images[region.Slot]; ok && region.Kind == pptx.RegionPicture {
			copied := placed
			converted.Image = &copied
		}
		regions = append(regions, converted)
	}
	writeData(writer, request, http.StatusOK, map[string]any{
		"slide": position, "layoutId": layout.ID, "layoutName": layout.Name,
		"aspectRatio": float64(width) / float64(height),
		// The slide's height in points, so a browser can size type the way the
		// renderer does: a size in points is a fraction of this, whatever pixel
		// width the canvas happens to be drawn at.
		"slideHeightPoints": float64(height) / float64(pptx.EMUPerPoint),
		"regions":           regions,
	})
}

type revisionRequest struct {
	Action      string `json:"action"`
	Instruction string `json:"instruction"`
	Slot        string `json:"slot"`
}

// reviseSlide asks the model for another draft of one slide and returns it
// without saving. The editor shows the result and the author decides.
//
// It is scoped to one slide on purpose. Regenerating a deck to fix one sentence
// throws away every edit made since, which is why people stop editing generated
// decks and start rewriting them elsewhere.
func (s *Server) reviseSlide(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	if s.generator == nil {
		writeError(writer, request, http.StatusServiceUnavailable, "ai_unavailable",
			"This deployment has no AI provider configured", nil)
		return
	}
	var input revisionRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	if utf8.RuneCountInString(input.Instruction) > 2000 || utf8.RuneCountInString(input.Slot) > 100 {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error",
			"instruction or slot exceeds its allowed length", nil)
		return
	}
	presentation, err := s.store.GetPresentation(request.Context(), request.PathValue("id"), user.ID, false)
	if err != nil {
		s.handleStoreError(writer, request, err, "presentation_read_failed")
		return
	}
	position, ok := slidePosition(writer, request, len(presentation.Slides))
	if !ok {
		return
	}
	_, manifest, err := s.presentationTemplate(request.Context(), presentation)
	if err != nil {
		s.handleStoreError(writer, request, err, "presentation_template_unavailable")
		return
	}
	target := presentation.Slides[position-1]
	single := presentation
	single.Slides = []model.Slide{target}
	source := deck.Format(single, manifest)
	profile, _ := s.store.GetProfile(request.Context(), user.ID)
	revision := generation.Revision{
		Presentation: presentation, Profile: profile,
		Template:    generation.Template{ID: templateIDOf(presentation), Name: "", Manifest: manifest},
		Source:      source,
		Action:      strings.ToLower(strings.TrimSpace(input.Action)),
		Instruction: input.Instruction,
		Focus:       strings.TrimSpace(input.Slot),
		DeckOutline: deckOutline(presentation, position),
	}
	// A rewrite asked to make a slide fit is given the measurements, not an
	// adjective: "0.9cm too tall" is actionable, "too long" is not.
	for _, finding := range s.inspectCompiled(request, user.ID, presentation, manifest, presentation.Slides) {
		if finding.Slide == position {
			revision.Findings = append(revision.Findings, finding.Slot+": "+finding.Detail)
		}
	}
	revised, err := s.generator.ReviseSlide(request.Context(), revision)
	if err != nil {
		if errors.Is(err, generation.ErrProviderUnavailable) {
			writeError(writer, request, http.StatusServiceUnavailable, "ai_unavailable",
				"This deployment has no AI provider configured", nil)
			return
		}
		writeError(writer, request, http.StatusBadGateway, "ai_revision_failed", err.Error(), nil)
		return
	}
	compiled := generation.CompileSourceWith(revised, single, profile,
		generation.Template{ID: templateIDOf(presentation), Manifest: manifest},
		s.resolveImage(request, user.ID), s.gridResolver(request, user.ID))
	if len(compiled.Slides) == 0 {
		writeError(writer, request, http.StatusBadGateway, "ai_revision_failed",
			"The revision could not be bound to this template", map[string]any{"source": revised})
		return
	}
	// The canvas layer belongs to the author, not to the model: objects they
	// placed and regions they moved survive a rewrite of the words.
	proposal := compiled.Slides[0]
	proposal.ID = target.ID
	proposal.Position = target.Position
	proposal.Content = carryCanvasLayer(proposal.Content, target.Content)
	preview := presentation
	preview.Slides = append([]model.Slide(nil), presentation.Slides...)
	preview.Slides[position-1] = proposal
	findings := s.inspectCompiled(request, user.ID, preview, manifest, preview.Slides)
	slideFindings := make([]pptx.Finding, 0, 2)
	for _, finding := range findings {
		if finding.Slide == position {
			slideFindings = append(slideFindings, finding)
		}
	}
	s.store.Audit(request.Context(), &user.ID, "presentation.slide_revise", "presentation", presentation.ID,
		map[string]any{"slide": position, "action": revision.Action})
	writeData(writer, request, http.StatusOK, map[string]any{
		"slide": position, "source": revised, "proposal": proposal,
		"warnings": compiled.Warnings, "findings": slideFindings, "applied": false,
	})
}

// carryCanvasLayer keeps the objects and moved regions of the stored slide on a
// slide the model just rewrote.
func carryCanvasLayer(proposed, stored []byte) []byte {
	if len(stored) == 0 {
		return proposed
	}
	existing := deck.Decode(stored)
	if len(existing.Elements) == 0 && len(existing.Frames) == 0 && len(existing.Styles) == 0 {
		return proposed
	}
	content := deck.Decode(proposed)
	content.Elements = existing.Elements
	content.Frames = existing.Frames
	content.Styles = existing.Styles
	return content.Encode()
}

// deckOutline is the deck around one slide, in one line each.
func deckOutline(presentation model.Presentation, position int) []string {
	outline := make([]string, 0, len(presentation.Slides))
	for index, slide := range presentation.Slides {
		marker := ""
		if index+1 == position {
			marker = " (this one)"
		}
		outline = append(outline, strconv.Itoa(index+1)+". "+strings.TrimSpace(slide.Title)+marker)
	}
	return outline
}

// slidePosition reads the slide number from the path and bounds it to the deck.
func slidePosition(writer http.ResponseWriter, request *http.Request, slides int) (int, bool) {
	position, err := strconv.Atoi(request.PathValue("position"))
	if err != nil || position < 1 || position > slides {
		writeError(writer, request, http.StatusNotFound, "not_found",
			"The requested slide does not exist", map[string]any{"slides": slides})
		return 0, false
	}
	return position, true
}
