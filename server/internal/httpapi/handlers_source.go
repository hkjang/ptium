package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"unicode/utf8"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/export"
	"github.com/hkjang/ptium/server/internal/generation"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
	"github.com/hkjang/ptium/server/internal/store"
)

// maximumSourceBytes bounds hand-written deck source. A fifty-slide deck runs to
// a few kilobytes; anything past this is not a deck.
const maximumSourceBytes = 256 << 10

// getPresentationSource returns the deck as text.
//
// The source is the deck: compiling it produces exactly the slides that are
// stored. A deck generated before sources were kept, or one edited on the canvas,
// is written back out from its slides so the text is always available and always
// current.
func (s *Server) getPresentationSource(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	presentation, err := s.store.GetPresentation(request.Context(), request.PathValue("id"), user.ID, false)
	if err != nil {
		s.handleStoreError(writer, request, err, "presentation_read_failed")
		return
	}
	_, manifest, err := s.presentationTemplate(request.Context(), presentation)
	if err != nil {
		s.handleStoreError(writer, request, err, "presentation_template_unavailable")
		return
	}
	source := presentation.Source
	// Slides are the authority: if they have been edited since the source was
	// written, the text is regenerated from them rather than shown out of date.
	if formatted := deck.Format(presentation, manifest); formatted != "" &&
		!deck.MatchesSlides(source, presentation, manifest) {
		source = formatted
	}
	writeData(writer, request, http.StatusOK, map[string]any{
		"source":     source,
		"slideCount": len(presentation.Slides),
		"language":   presentation.Language,
		"blockKinds": pptx.BlockKinds(),
		"layouts":    sourceLayouts(manifest),
	})
}

type sourceRequest struct {
	Source string `json:"source"`
	// DryRun compiles and reports without storing anything, which is what a live
	// preview needs.
	DryRun bool `json:"dryRun"`
	// Version protects an applied source from overwriting a newer canvas edit.
	// It is optional for backwards compatibility with older API clients.
	Version *int64 `json:"version"`
}

// putPresentationSource compiles hand-written source and replaces the deck.
func (s *Server) putPresentationSource(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	var input sourceRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	if len(input.Source) > maximumSourceBytes {
		writeError(writer, request, http.StatusUnprocessableEntity, "source_too_large",
			"The deck source is larger than this deployment accepts", nil)
		return
	}
	if !utf8.ValidString(input.Source) {
		writeError(writer, request, http.StatusUnprocessableEntity, "invalid_source", "The deck source must be UTF-8 text", nil)
		return
	}
	presentation, err := s.store.GetPresentation(request.Context(), request.PathValue("id"), user.ID, false)
	if err != nil {
		s.handleStoreError(writer, request, err, "presentation_read_failed")
		return
	}
	_, manifest, err := s.presentationTemplate(request.Context(), presentation)
	if err != nil {
		s.handleStoreError(writer, request, err, "presentation_template_unavailable")
		return
	}
	profile, _ := s.store.GetProfile(request.Context(), user.ID)
	compiled := generation.CompileSourceWith(input.Source, presentation, profile,
		generation.Template{ID: templateIDOf(presentation), Manifest: manifest},
		s.resolveImage(request, user.ID), s.gridResolver(request, user.ID))
	if len(compiled.Slides) == 0 {
		writeError(writer, request, http.StatusUnprocessableEntity, "empty_source",
			"The deck source produced no slides", map[string]any{"warnings": compiled.Warnings})
		return
	}
	if allowed := s.maximumSlides(request.Context()); len(compiled.Slides) > allowed {
		// Both numbers, because the author has one of them and needs the other:
		// "more than this deployment allows" leaves somebody counting their own
		// slides to work out how many to cut.
		writeError(writer, request, http.StatusUnprocessableEntity, "too_many_slides",
			fmt.Sprintf("The deck source produced %d slides; this deployment allows %d",
				len(compiled.Slides), allowed),
			map[string]any{"slides": len(compiled.Slides), "allowed": allowed})
		return
	}
	// The canvas layer belongs to the author, not to the text: objects they placed
	// and regions they moved survive a re-compile of the words, as long as the
	// slide they sit on can still be identified.
	compiled.Slides = carryCanvasLayerAcross(presentation.Slides, compiled.Slides)
	findings := s.inspectCompiled(request, user.ID, presentation, manifest, compiled.Slides)
	if input.DryRun {
		writeData(writer, request, http.StatusOK, map[string]any{
			"slides": compiled.Slides, "outline": compiled.Outline, "warnings": compiled.Warnings,
			"findings": findings, "applied": false,
		})
		return
	}
	if err := s.store.ReplaceSlidesFromSource(request.Context(), presentation.ID, user.ID,
		input.Source, compiled.Outline, compiled.Slides, input.Version); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(writer, request, http.StatusNotFound, "not_found", "The presentation does not exist", nil)
			return
		}
		if errors.Is(err, store.ErrConflict) {
			writeError(writer, request, http.StatusConflict, "version_conflict", "The presentation changed in another session", nil)
			return
		}
		s.internalError(writer, request, "presentation_source_save_failed", err)
		return
	}
	s.store.Audit(request.Context(), &user.ID, "presentation.source_update", "presentation", presentation.ID, nil)
	updated, err := s.store.GetPresentation(request.Context(), presentation.ID, user.ID, false)
	if err != nil {
		s.handleStoreError(writer, request, err, "presentation_read_failed")
		return
	}
	writeData(writer, request, http.StatusOK, map[string]any{
		"presentation": updated, "warnings": compiled.Warnings, "findings": findings, "applied": true,
	})
}

// previewSource renders one slide of source that has not been saved.
//
// This is what makes writing a deck as text feel like writing code: the slide
// appears as it is typed, drawn through the real template, without the deck
// having to be replaced first.
func (s *Server) previewSource(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	var input sourceRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	if len(input.Source) > maximumSourceBytes || !utf8.ValidString(input.Source) {
		writeError(writer, request, http.StatusUnprocessableEntity, "invalid_source",
			"The deck source must be UTF-8 text within the size limit", nil)
		return
	}
	presentation, err := s.store.GetPresentation(request.Context(), request.PathValue("id"), user.ID, false)
	if err != nil {
		s.handleStoreError(writer, request, err, "presentation_read_failed")
		return
	}
	data, manifest, err := s.presentationTemplate(request.Context(), presentation)
	if err != nil {
		s.handleStoreError(writer, request, err, "presentation_template_unavailable")
		return
	}
	compiled := deck.Compile(deck.ParseSource(input.Source), manifest, deck.CompileOptions{
		Language: presentation.Language, ResolveImage: s.resolveImage(request, user.ID),
		ResolveGrid: s.gridResolver(request, user.ID)})
	if len(compiled.Slides) == 0 {
		writeError(writer, request, http.StatusUnprocessableEntity, "empty_source", "The deck source produced no slides", nil)
		return
	}
	position, _ := strconv.Atoi(request.URL.Query().Get("slide"))
	if position < 1 {
		position = 1
	}
	if position > len(compiled.Slides) {
		position = len(compiled.Slides)
	}
	// The compiled slides are rendered directly, so nothing is stored and the
	// deck on screen is untouched until the author applies it.
	preview := model.Presentation{
		ID: presentation.ID, Title: presentation.Title, Language: presentation.Language,
		Theme: presentation.Theme, TemplateID: presentation.TemplateID, Slides: compiled.Slides,
	}
	svg, err := export.PreviewSVG(preview, manifest, position, previewWidth(request),
		templateMedia(data), s.imageSource(request, user.ID))
	if err != nil {
		writeError(writer, request, http.StatusUnprocessableEntity, "preview_failed", err.Error(), nil)
		return
	}
	writer.Header().Set("X-Ptium-Slide-Count", strconv.Itoa(len(compiled.Slides)))
	writeSVG(writer, svg)
}

// inspectCompiled measures the drawn slides: text that will not fit, something
// drawn off the slide, a collision, unreadable contrast. A warning says what
// compiling adjusted; a finding says what still looks wrong once it is drawn.
func (s *Server) inspectCompiled(request *http.Request, ownerID string,
	presentation model.Presentation, manifest pptx.Manifest, slides []model.Slide) []pptx.Finding {
	if len(slides) == 0 {
		return nil
	}
	copied := presentation
	copied.Slides = slides
	built := deck.BuildWithImages(copied, manifest, "", s.imageSource(request, ownerID))
	return pptx.InspectDeck(manifest, built)
}

// carryCanvasLayerAcross moves the freeform objects and moved regions of the
// stored deck onto the deck the source just produced.
//
// Slides are matched by position when the source still has the same number of
// them, which is what editing prose in the code view does. When the count
// changed, only a slide whose title is unchanged keeps its objects: putting them
// on a slide that is now about something else is worse than dropping them.
func carryCanvasLayerAcross(stored, compiled []model.Slide) []model.Slide {
	byTitle := map[string]model.Slide{}
	for _, slide := range stored {
		if _, clash := byTitle[slide.Title]; clash {
			// An ambiguous title identifies nothing.
			byTitle[slide.Title] = model.Slide{}
			continue
		}
		byTitle[slide.Title] = slide
	}
	sameLength := len(stored) == len(compiled)
	for index := range compiled {
		source := model.Slide{}
		if sameLength {
			source = stored[index]
		} else if matched, ok := byTitle[compiled[index].Title]; ok {
			source = matched
		}
		if len(source.Content) == 0 {
			continue
		}
		compiled[index].Content = carryCanvasLayer(compiled[index].Content, source.Content)
	}
	return compiled
}

// sourceMatchesSlides reports whether stored source still describes the stored
// slides, by compiling it and writing both back out as source.
//
// Comparing only titles and layouts was not enough. A slide can be edited on the
// canvas, rewritten by the model, or given speaker notes by a safe fix, and none
// of that changes its title — so the stored text stayed "current" while the deck
// had moved on. The workspace then showed the old text as though it were the
// deck, and applying it would have thrown the newer edits away.
//
// Formatting is the source language's own inverse, so comparing the two
// formatted forms compares exactly what the text can express: the points, the
// components, the images, the notes — and nothing the text was never meant to
// carry, such as a canvas layer.

// sourceLayouts is the layout vocabulary an author can write in @layout.
func sourceLayouts(manifest pptx.Manifest) []map[string]string {
	result := make([]map[string]string, 0, len(manifest.Layouts))
	for _, layout := range manifest.Layouts {
		result = append(result, map[string]string{"id": layout.ID, "name": layout.Name, "role": layout.Role})
	}
	return result
}

func templateIDOf(presentation model.Presentation) string {
	if presentation.TemplateID != nil {
		return *presentation.TemplateID
	}
	return ""
}

// inspectPresentation measures a stored deck as it will be drawn.
//
// It answers the question a person would otherwise answer by exporting the file
// and looking at every slide: does anything overflow, collide, fall off the edge
// or become unreadable.
func (s *Server) inspectPresentation(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	presentation, err := s.store.GetPresentation(request.Context(), request.PathValue("id"), user.ID, false)
	if err != nil {
		s.handleStoreError(writer, request, err, "presentation_read_failed")
		return
	}
	if len(presentation.Slides) == 0 {
		writeError(writer, request, http.StatusConflict, "presentation_has_no_slides",
			"Generate or add slides before inspecting", nil)
		return
	}
	_, manifest, err := s.presentationTemplate(request.Context(), presentation)
	if err != nil {
		s.handleStoreError(writer, request, err, "presentation_template_unavailable")
		return
	}
	findings := s.inspectCompiled(request, user.ID, presentation, manifest, presentation.Slides)
	defects := pptx.Defects(findings)
	writeData(writer, request, http.StatusOK, map[string]any{
		"slides": len(presentation.Slides), "findings": findings,
		// clean is about the drawing. A deck can be drawn correctly and still be
		// unfinished, and saying so in one boolean would hide both.
		"clean": len(defects) == 0, "defects": len(defects), "advisories": len(findings) - len(defects),
		// The same measurements, scored: a list answers "what should I fix" and a
		// score answers "is this ready", which is the question people ask first.
		"score": pptx.ScoreDeck(findings, len(presentation.Slides)),
	})
}
