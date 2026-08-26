package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hkjang/ptium/server/internal/export"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
	"github.com/hkjang/ptium/server/internal/store"
)

// A deck is written to be shown to someone. Until now the only way to show one
// to a person without an account here was to export the file and send it, and a
// deck that leaves as a file stops being the deck in Ptium and becomes four
// copies in four inboxes — none of which gets the correction made after they
// were sent.
//
// A share is a link that opens one deck, read-only, for whoever holds it. It
// carries no session and grants nothing else: not the deck's source, not its
// template, not the workspace it lives in. The owner can revoke it, and can
// give it a date after which it stops working.

type shareRequest struct {
	Label string `json:"label"`
	// Days is how long the link stays open. Zero leaves it open until it is
	// revoked, which is what a deck shared inside a team usually wants.
	Days int `json:"days"`
}

func (s *Server) createShare(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	var input shareRequest
	if request.ContentLength > 0 && !decodeJSON(writer, request, &input) {
		return
	}
	if utf8.RuneCountInString(input.Label) > 120 || input.Days < 0 || input.Days > 3650 {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error",
			"a share needs a short label and a sensible number of days", nil)
		return
	}
	var expires *time.Time
	if input.Days > 0 {
		when := time.Now().AddDate(0, 0, input.Days)
		expires = &when
	}
	share, token, err := s.store.CreateShare(request.Context(), request.PathValue("id"), user.ID, input.Label, expires)
	if err != nil {
		s.handleStoreError(writer, request, err, "share_create_failed")
		return
	}
	// The link is shown once. Nothing stored can produce it again, which is the
	// point: a lost database is not a set of open decks.
	share.URL = shareURL(request, token)
	writeData(writer, request, http.StatusCreated, share)
}

func (s *Server) listShares(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	shares, err := s.store.ListShares(request.Context(), request.PathValue("id"), user.ID)
	if err != nil {
		s.handleStoreError(writer, request, err, "share_list_failed")
		return
	}
	writeData(writer, request, http.StatusOK, shares)
}

func (s *Server) revokeShare(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	if err := s.store.RevokeShare(request.Context(), request.PathValue("id"), user.ID, request.PathValue("shareId")); err != nil {
		s.handleStoreError(writer, request, err, "share_revoke_failed")
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

// sharedDeck is what a link shows: the slides, and the deck's own name. Not the
// source, not the template, not who made it.
type sharedDeck struct {
	Title      string       `json:"title"`
	SlideCount int          `json:"slideCount"`
	Slides     []sharedPage `json:"slides"`
	// Titles is what the first viewer shipped with, kept so a page served from
	// a cache keeps working against a newer server.
	Titles   []string `json:"titles"`
	Language string   `json:"language,omitempty"`
}

// sharedPage is one slide as a link sees it: what it is called, and the id a
// comment attaches itself to.
type sharedPage struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

func (s *Server) sharedPresentation(writer http.ResponseWriter, request *http.Request) {
	presentation, err := s.presentationFromShare(writer, request)
	if err != nil {
		return
	}
	shown := shownSlides(presentation)
	titles := make([]string, 0, len(shown))
	pages := make([]sharedPage, 0, len(shown))
	for _, slide := range shown {
		titles = append(titles, slide.Title)
		pages = append(pages, sharedPage{ID: slide.ID, Title: slide.Title})
	}
	writeData(writer, request, http.StatusOK, sharedDeck{
		Title: presentation.Title, SlideCount: len(shown),
		Slides: pages, Titles: titles, Language: presentation.Language,
	})
}

func (s *Server) sharedPreview(writer http.ResponseWriter, request *http.Request) {
	presentation, err := s.presentationFromShare(writer, request)
	if err != nil {
		return
	}
	shown := shownSlides(presentation)
	if len(shown) == 0 {
		writeError(writer, request, http.StatusConflict, "presentation_has_no_slides", "This deck has no slides yet", nil)
		return
	}
	data, manifest, err := s.presentationTemplate(request.Context(), presentation)
	if err != nil {
		s.handleStoreError(writer, request, err, "presentation_template_unavailable")
		return
	}
	position, _ := strconv.Atoi(request.URL.Query().Get("slide"))
	if position < 1 {
		position = 1
	}
	if position > len(shown) {
		position = len(shown)
	}
	// The link counts the slides that are part of the show, and draws that one.
	position = shown[position-1].Position
	options := pptx.PreviewOptions{Width: previewWidth(request), Media: templateMedia(data),
		Reveal: revealCount(request)}
	// Drawn exactly as the editor draws it, images and all — a link shows the
	// deck the owner sees, not a reduced copy of it.
	svg, err := export.PreviewSlideSVG(presentation, manifest, position, options,
		s.imageSource(request, presentation.OwnerID))
	if err != nil {
		writeError(writer, request, http.StatusNotFound, "not_found", err.Error(), nil)
		return
	}
	writeSVG(writer, svg)
}

// shownSlides are the slides a link is for.
//
// A slide marked skipped is one the author took out of the talk: PowerPoint
// hides it, the handout leaves it out, and a link handed to somebody else was
// drawing it — title, points and all — to anyone who had the link.
func shownSlides(presentation model.Presentation) []model.Slide {
	shown := make([]model.Slide, 0, len(presentation.Slides))
	for _, slide := range presentation.Slides {
		if slideIsSkipped(slide) {
			continue
		}
		shown = append(shown, slide)
	}
	return shown
}

// slideIsSkipped reads the one field of the stored content this needs.
func slideIsSkipped(slide model.Slide) bool {
	var marked struct {
		Skipped bool `json:"skipped"`
	}
	if len(slide.Content) == 0 {
		return false
	}
	_ = json.Unmarshal(slide.Content, &marked)
	return marked.Skipped
}

func (s *Server) presentationFromShare(writer http.ResponseWriter, request *http.Request) (model.Presentation, error) {
	token := strings.TrimSpace(request.PathValue("token"))
	presentation, err := s.store.PresentationByShare(request.Context(), token)
	switch {
	case errors.Is(err, store.ErrShareClosed):
		writeError(writer, request, http.StatusGone, "share_closed",
			"This link is no longer open. Ask whoever sent it for a new one", nil)
		return model.Presentation{}, err
	case err != nil:
		writeError(writer, request, http.StatusNotFound, "not_found", "No deck is shared at this link", nil)
		return model.Presentation{}, err
	}
	return presentation, nil
}

// A link lets someone look at a deck. Looking is half of a review; the other
// half is saying what is wrong with slide 4. These are the two ends of that:
// whoever holds the link can leave a remark and read the ones already left, and
// the owner sees them all in the workspace and marks them dealt with.

type commentRequest struct {
	SlideID string `json:"slideId"`
	Author  string `json:"author"`
	Body    string `json:"body"`
	// ParentID answers a remark rather than adding one. Whoever holds the link
	// may answer: half of a review is the reviewer being told what happened to
	// what they said.
	ParentID string `json:"parentId"`
}

func (s *Server) addSharedComment(writer http.ResponseWriter, request *http.Request) {
	presentation, err := s.presentationFromShare(writer, request)
	if err != nil {
		return
	}
	var input commentRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	// The slide has to be one of this deck's: a link to one deck is not a way to
	// write on another.
	if strings.TrimSpace(input.SlideID) != "" && !slideBelongsTo(presentation, input.SlideID) {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error",
			"that slide is not part of this deck", nil)
		return
	}
	comment, err := s.store.AddComment(request.Context(), presentation.ID, store.CommentInput{
		SlideID: strings.TrimSpace(input.SlideID), Author: input.Author, Body: input.Body,
		ParentID: strings.TrimSpace(input.ParentID),
	})
	switch {
	case errors.Is(err, store.ErrTooManyComments):
		writeError(writer, request, http.StatusTooManyRequests, "too_many_comments", err.Error(), nil)
		return
	case err != nil:
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", err.Error(), nil)
		return
	}
	writeData(writer, request, http.StatusCreated, comment)
}

func (s *Server) sharedComments(writer http.ResponseWriter, request *http.Request) {
	presentation, err := s.presentationFromShare(writer, request)
	if err != nil {
		return
	}
	comments, err := s.store.Comments(request.Context(), presentation.ID)
	if err != nil {
		s.handleStoreError(writer, request, err, "comment_list_failed")
		return
	}
	writeData(writer, request, http.StatusOK, comments)
}

func (s *Server) listComments(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	comments, err := s.store.OwnerComments(request.Context(), request.PathValue("id"), user.ID)
	if err != nil {
		s.handleStoreError(writer, request, err, "comment_list_failed")
		return
	}
	writeData(writer, request, http.StatusOK, comments)
}

// addOwnerComment is the other half of a review: the author answering.
//
// Until now the only way a remark could be answered was from the link, by
// somebody with no account — so the person who fixed the slide had nowhere to
// say so, and a reviewer holding the link never learned what happened to what
// they said. Resolving is a state; "고쳤습니다, 4번은 그대로 둡니다" is a
// sentence, and reviews run on sentences.
func (s *Server) addOwnerComment(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	var input commentRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	presentation, err := s.store.GetPresentation(request.Context(), request.PathValue("id"), user.ID, false)
	if err != nil {
		s.handleStoreError(writer, request, err, "presentation_read_failed")
		return
	}
	if strings.TrimSpace(input.SlideID) != "" && !slideBelongsTo(presentation, input.SlideID) {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error",
			"that slide is not part of this deck", nil)
		return
	}
	author := strings.TrimSpace(input.Author)
	if author == "" {
		author = strings.TrimSpace(user.Name)
	}
	if author == "" {
		author = user.Email
	}
	comment, err := s.store.AddComment(request.Context(), presentation.ID, store.CommentInput{
		SlideID: strings.TrimSpace(input.SlideID), Author: author, Body: input.Body,
		ParentID: strings.TrimSpace(input.ParentID),
	})
	switch {
	case errors.Is(err, store.ErrTooManyComments):
		writeError(writer, request, http.StatusTooManyRequests, "too_many_comments", err.Error(), nil)
		return
	case err != nil:
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", err.Error(), nil)
		return
	}
	writeData(writer, request, http.StatusCreated, comment)
}

func (s *Server) resolveComment(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	var input struct {
		Resolved *bool `json:"resolved"`
	}
	if request.ContentLength > 0 && !decodeJSON(writer, request, &input) {
		return
	}
	resolved := true
	if input.Resolved != nil {
		resolved = *input.Resolved
	}
	if err := s.store.ResolveComment(request.Context(), request.PathValue("id"), user.ID,
		request.PathValue("commentId"), resolved); err != nil {
		s.handleStoreError(writer, request, err, "comment_resolve_failed")
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteComment(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	if err := s.store.DeleteComment(request.Context(), request.PathValue("id"), user.ID,
		request.PathValue("commentId")); err != nil {
		s.handleStoreError(writer, request, err, "comment_delete_failed")
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

// slideBelongsTo reports whether this deck has that slide.
func slideBelongsTo(presentation model.Presentation, slideID string) bool {
	for _, slide := range presentation.Slides {
		if slide.ID == slideID {
			return true
		}
	}
	return false
}

// shareURL is the address to hand to a person, built from the request so it is
// right behind a reverse proxy as well as in front of one.
func shareURL(request *http.Request, token string) string {
	scheme := "https"
	if request.TLS == nil && request.Header.Get("X-Forwarded-Proto") == "" {
		scheme = "http"
	}
	if forwarded := request.Header.Get("X-Forwarded-Proto"); forwarded != "" {
		scheme = forwarded
	}
	host := request.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = request.Host
	}
	return scheme + "://" + host + "/view/" + token
}
