package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/export"
	"github.com/hkjang/ptium/server/internal/generation"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
	"github.com/hkjang/ptium/server/internal/store"
)

// Slides someone keeps.
//
// Every deck a person makes has pages that are not really about this deck: the
// company introduction, the team, how to reach us, the legal notice. Rebuilding
// those each time is the least interesting work in the product.
//
// A saved slide is stored as deck source rather than as a drawn slide, so
// inserting it lays it out in the template of the deck it lands in. Pasting a
// picture of someone else's design into a deck is exactly what nobody wants.

// listSnippets returns the caller's saved slides.
func (s *Server) listSnippets(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	limit, offset := pagination(request)
	query := request.URL.Query()
	items, total, err := s.store.ListSnippets(request.Context(), user.ID, store.SnippetQuery{
		Search:   searchTerm(request),
		Tag:      query.Get("tag"),
		Favorite: query.Get("favorite") == "true",
		Sort:     query.Get("sort"),
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		s.internalError(writer, request, "snippets_read_failed", err)
		return
	}
	writeList(writer, request, items, total, limit, offset)
}

// listSnippetTags returns the words the caller files saved slides under.
func (s *Server) listSnippetTags(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	tags, err := s.store.SnippetTags(request.Context(), user.ID)
	if err != nil {
		s.internalError(writer, request, "snippet_tags_read_failed", err)
		return
	}
	writeData(writer, request, http.StatusOK, tags)
}

type snippetRequest struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
	// Source is the slide as text. A client that has a slide rather than text
	// sends where it is instead, and the server writes the text.
	Source         string `json:"source"`
	PresentationID string `json:"presentationId"`
	Slide          int    `json:"slide"`
}

// createSnippet saves a slide for reuse.
func (s *Server) createSnippet(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	var input snippetRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	source := strings.TrimSpace(input.Source)
	role := ""
	if source == "" {
		// Saved from the editor: the deck knows how to write itself down, and one
		// slide of that text is the snippet.
		if strings.TrimSpace(input.PresentationID) == "" || input.Slide < 1 {
			writeError(writer, request, http.StatusUnprocessableEntity, "validation_error",
				"Send either the slide's source, or the deck and the slide to save", nil)
			return
		}
		written, slideRole, err := s.slideAsSource(request, user.ID, input.PresentationID, input.Slide)
		if err != nil {
			s.handleStoreError(writer, request, err, "presentation_read_failed")
			return
		}
		source, role = written, slideRole
	}
	if !utf8.ValidString(source) {
		writeError(writer, request, http.StatusUnprocessableEntity, "invalid_source", "A saved slide must be UTF-8 text", nil)
		return
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = snippetName(source)
	}
	snippet, err := s.store.CreateSnippet(request.Context(), user.ID, store.SnippetInput{
		Name: name, Source: source, Role: role, Tags: input.Tags,
	})
	if err != nil {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", err.Error(), nil)
		return
	}
	s.store.Audit(request.Context(), &user.ID, "snippet.create", "snippet", snippet.ID, nil)
	writeData(writer, request, http.StatusCreated, snippet)
}

func (s *Server) getSnippet(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	snippet, err := s.store.GetSnippet(request.Context(), request.PathValue("id"), user.ID)
	if err != nil {
		s.handleStoreError(writer, request, err, "snippet_read_failed")
		return
	}
	writeData(writer, request, http.StatusOK, snippet)
}

func (s *Server) patchSnippet(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	var body struct {
		Name   *string   `json:"name"`
		Source *string   `json:"source"`
		Tags   *[]string `json:"tags"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}
	snippet, err := s.store.UpdateSnippet(request.Context(), request.PathValue("id"), user.ID,
		store.SnippetPatch{Name: body.Name, Source: body.Source, Tags: body.Tags})
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeError(writer, request, http.StatusConflict, "snippet_name_taken",
				"이미 같은 이름으로 저장한 슬라이드가 있습니다", nil)
			return
		}
		if errors.Is(err, store.ErrNotFound) {
			writeError(writer, request, http.StatusNotFound, "not_found", "The requested resource was not found", nil)
			return
		}
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", err.Error(), nil)
		return
	}
	writeData(writer, request, http.StatusOK, snippet)
}

func (s *Server) deleteSnippet(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	if err := s.store.DeleteSnippet(request.Context(), request.PathValue("id"), user.ID); err != nil {
		s.handleStoreError(writer, request, err, "snippet_delete_failed")
		return
	}
	s.store.Audit(request.Context(), &user.ID, "snippet.delete", "snippet", request.PathValue("id"), nil)
	writer.WriteHeader(http.StatusNoContent)
}

func (s *Server) favoriteSnippet(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	var body struct {
		Favorite bool `json:"favorite"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}
	id := request.PathValue("id")
	if _, err := s.store.GetSnippet(request.Context(), id, user.ID); err != nil {
		s.handleStoreError(writer, request, err, "snippet_read_failed")
		return
	}
	if err := s.store.SetFavorite(request.Context(), user.ID, store.FavoriteSnippet, id, body.Favorite); err != nil {
		s.internalError(writer, request, "snippet_favorite_failed", err)
		return
	}
	snippet, err := s.store.GetSnippet(request.Context(), id, user.ID)
	if err != nil {
		s.handleStoreError(writer, request, err, "snippet_read_failed")
		return
	}
	writeData(writer, request, http.StatusOK, snippet)
}

// renderSnippet compiles a saved slide into a deck's template and hands back the
// slide, ready to be inserted.
//
// The compile happens here rather than in the browser because that is where the
// template lives: the same saved page becomes a two-column slide in one design
// and a full-bleed one in another, which is the whole point of storing text.
func (s *Server) renderSnippet(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	var body struct {
		PresentationID string `json:"presentationId"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}
	snippet, err := s.store.GetSnippet(request.Context(), request.PathValue("id"), user.ID)
	if err != nil {
		s.handleStoreError(writer, request, err, "snippet_read_failed")
		return
	}
	presentation, manifest, err := s.snippetTarget(request, user.ID, body.PresentationID)
	if err != nil {
		s.handleStoreError(writer, request, err, "presentation_read_failed")
		return
	}
	compiled := s.compileSnippet(request, user.ID, snippet, presentation, manifest)
	if len(compiled.Slides) == 0 {
		writeError(writer, request, http.StatusUnprocessableEntity, "empty_snippet",
			"The saved slide produced nothing", map[string]any{"warnings": compiled.Warnings})
		return
	}
	s.store.MarkSnippetUsed(request.Context(), snippet.ID, user.ID)
	writeData(writer, request, http.StatusOK, map[string]any{
		"slide": compiled.Slides[0], "warnings": compiled.Warnings, "name": snippet.Name,
	})
}

// snippetPreview draws a saved slide in a deck's template, so someone choosing
// one sees what they are about to insert rather than a name in a list.
func (s *Server) snippetPreview(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	snippet, err := s.store.GetSnippet(request.Context(), request.PathValue("id"), user.ID)
	if err != nil {
		s.handleStoreError(writer, request, err, "snippet_read_failed")
		return
	}
	presentation, manifest, err := s.snippetTarget(request, user.ID, request.URL.Query().Get("presentationId"))
	if err != nil {
		s.handleStoreError(writer, request, err, "presentation_read_failed")
		return
	}
	compiled := s.compileSnippet(request, user.ID, snippet, presentation, manifest)
	if len(compiled.Slides) == 0 {
		writeError(writer, request, http.StatusNotFound, "not_found", "The saved slide produced nothing", nil)
		return
	}
	preview := presentation
	preview.Slides = compiled.Slides[:1]
	data, _, _ := s.presentationTemplate(request.Context(), presentation)
	svg, err := export.PreviewSlideSVG(preview, manifest, 1,
		pptx.PreviewOptions{Width: previewWidth(request), Media: templateMedia(data)},
		s.imageSource(request, user.ID))
	if err != nil {
		writeError(writer, request, http.StatusNotFound, "not_found", err.Error(), nil)
		return
	}
	writeSVG(writer, svg)
}

// snippetTarget is the deck a saved slide is being read into, and its template.
// Without a deck — browsing the library on its own — the person's default design
// stands in, so a preview is still a real drawing.
func (s *Server) snippetTarget(request *http.Request, ownerID, presentationID string) (model.Presentation, pptx.Manifest, error) {
	if strings.TrimSpace(presentationID) != "" {
		presentation, err := s.store.GetPresentation(request.Context(), presentationID, ownerID, false)
		if err != nil {
			return model.Presentation{}, pptx.Manifest{}, err
		}
		_, manifest, err := s.presentationTemplate(request.Context(), presentation)
		return presentation, manifest, err
	}
	// No deck named: the person's default design stands in, so the preview is
	// still a real drawing rather than a name in a list.
	presentation := model.Presentation{OwnerID: ownerID, Language: "ko"}
	_, manifest, err := s.presentationTemplate(request.Context(), presentation)
	return presentation, manifest, err
}

// compileSnippet turns one saved slide's source into a slide of the target deck.
func (s *Server) compileSnippet(request *http.Request, ownerID string, snippet model.Snippet,
	presentation model.Presentation, manifest pptx.Manifest) generation.Deck {
	profile, _ := s.store.GetProfile(request.Context(), ownerID)
	// The snippet is one slide, and the deck around it is not its business: it is
	// compiled on its own so its position in the target deck cannot change how it
	// is laid out.
	single := presentation
	single.Slides = nil
	return generation.CompileSourceWith(snippet.Source, single, profile,
		generation.Template{ID: templateIDOf(presentation), Manifest: manifest},
		s.resolveImage(request, ownerID), s.gridResolver(request, ownerID))
}

// slideAsSource writes one slide of a deck down as text.
func (s *Server) slideAsSource(request *http.Request, ownerID, presentationID string, position int) (string, string, error) {
	presentation, err := s.store.GetPresentation(request.Context(), presentationID, ownerID, false)
	if err != nil {
		return "", "", err
	}
	if position < 1 || position > len(presentation.Slides) {
		return "", "", store.ErrNotFound
	}
	_, manifest, err := s.presentationTemplate(request.Context(), presentation)
	if err != nil {
		return "", "", err
	}
	slide := presentation.Slides[position-1]
	single := presentation
	single.Slides = []model.Slide{slide}
	return deck.Format(single, manifest), strings.TrimSpace(slide.Layout), nil
}

// snippetName falls back to the slide's own title when nobody typed a name.
func snippetName(source string) string {
	for _, line := range strings.Split(source, "\n") {
		if title, ok := strings.CutPrefix(strings.TrimSpace(line), "# "); ok {
			if trimmed := strings.TrimSpace(title); trimmed != "" {
				return trimmed
			}
		}
	}
	return "저장한 슬라이드"
}
