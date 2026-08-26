package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/hkjang/ptium/server/internal/copilot"
	"github.com/hkjang/ptium/server/internal/pptx"
	"github.com/hkjang/ptium/server/internal/store"
)

// Telling the deck what to do, in words.
//
// The value is not a chat window; it is the translation from a sentence to an
// edit. "3번과 4번 합쳐줘" has one right answer, and reading it is faster and
// more reliable than asking a model to guess at it — which also means it works
// in a deployment with no model at all.
//
// Nothing is applied until the caller has been told what will happen: the plan
// comes back first, in the language it was typed in, and a dry run is the
// default a careful client uses.

type commandRequest struct {
	Text string `json:"text"`
	// DryRun asks what would happen without changing anything.
	DryRun bool `json:"dryRun"`
}

func (s *Server) runPresentationCommand(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	var input commandRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	presentation, err := s.store.GetPresentation(request.Context(), request.PathValue("id"), user.ID, false)
	if err != nil {
		s.handleStoreError(writer, request, err, "presentation_read_failed")
		return
	}
	if len(presentation.Slides) == 0 {
		writeError(writer, request, http.StatusConflict, "presentation_has_no_slides",
			"Generate or add slides before commanding the deck", nil)
		return
	}
	commands, err := copilot.Parse(input.Text, len(presentation.Slides))
	if err != nil {
		// Three different answers, and they used to be one. A sentence naming a
		// slide the deck does not have was understood; so was one asking for
		// what the deck already is. Telling their authors that nothing they
		// said made sense sends them looking for better words rather than for
		// the right number.
		var nothing copilot.ErrNothingToDo
		if errors.As(err, &nothing) {
			writeData(writer, request, http.StatusOK, map[string]any{
				"plan": []copilot.Command{}, "notes": []string{nothing.Reason}, "applied": false,
				"slides": len(presentation.Slides), "slidesAfter": len(presentation.Slides),
			})
			return
		}
		var beyond copilot.ErrOutOfRange
		if errors.As(err, &beyond) {
			writeError(writer, request, http.StatusUnprocessableEntity, "command_out_of_range",
				beyond.Error(), map[string]any{"text": strings.TrimSpace(input.Text),
					"slides": beyond.Slides, "position": beyond.Position})
			return
		}
		writeError(writer, request, http.StatusUnprocessableEntity, "command_not_understood",
			"이 문장에서 할 일을 찾지 못했습니다. 예: 3번과 4번 합쳐줘 · 5번 삭제 · 8장으로 줄여줘",
			map[string]any{"text": strings.TrimSpace(input.Text)})
		return
	}

	// A trim drops what measured worst, so the measurement is taken first — the
	// same one the workspace shows as the deck's score.
	weakest := func(position int) int { return 100 }
	if _, manifest, err := s.presentationTemplate(request.Context(), presentation); err == nil {
		findings := s.inspectCompiled(request, user.ID, presentation, manifest, presentation.Slides)
		score := pptx.ScoreDeck(findings, len(presentation.Slides))
		byPosition := map[int]int{}
		for _, slide := range score.Slides {
			byPosition[slide.Slide] = slide.Score
		}
		weakest = func(position int) int {
			if value, ok := byPosition[position]; ok {
				return value
			}
			return 100
		}
	}

	updated, notes, err := copilot.Apply(presentation.Slides, commands, weakest)
	if err != nil {
		writeError(writer, request, http.StatusUnprocessableEntity, "command_failed", err.Error(), nil)
		return
	}
	if notes == nil {
		notes = []string{}
	}
	if input.DryRun {
		writeData(writer, request, http.StatusOK, map[string]any{
			"plan": commands, "notes": notes, "applied": false,
			"slides": len(presentation.Slides), "slidesAfter": len(updated),
		})
		return
	}

	stored, err := s.store.UpdatePresentationWithSlides(request.Context(), presentation.ID, user.ID, false,
		store.PresentationInput{
			Title: presentation.Title, Prompt: presentation.Prompt, Theme: presentation.Theme,
			Language: presentation.Language, Audience: presentation.Audience, Tone: presentation.Tone,
			SlideCount: len(updated), TemplateID: presentation.TemplateID,
		}, &updated, nil)
	if err != nil {
		if errors.Is(err, store.ErrValidation) {
			writeError(writer, request, http.StatusUnprocessableEntity, "command_failed", err.Error(), nil)
			return
		}
		s.handleStoreError(writer, request, err, "presentation_update_failed")
		return
	}
	// The stored source is left alone on purpose: it no longer describes these
	// slides, and the workspace already writes the deck's own text back out when
	// that is true. One rule about which of the two is the deck, in one place.
	s.store.Audit(request.Context(), &user.ID, "presentation.command", "presentation", stored.ID,
		map[string]any{"text": strings.TrimSpace(input.Text), "kind": commands[0].Kind})
	writeData(writer, request, http.StatusOK, map[string]any{
		"plan": commands, "notes": notes, "applied": true,
		"presentation": stored, "slides": len(presentation.Slides), "slidesAfter": len(updated),
	})
}
