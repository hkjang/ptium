package httpapi

import (
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
	Title      string   `json:"title"`
	SlideCount int      `json:"slideCount"`
	Titles     []string `json:"titles"`
	Language   string   `json:"language,omitempty"`
}

func (s *Server) sharedPresentation(writer http.ResponseWriter, request *http.Request) {
	presentation, err := s.presentationFromShare(writer, request)
	if err != nil {
		return
	}
	titles := make([]string, 0, len(presentation.Slides))
	for _, slide := range presentation.Slides {
		titles = append(titles, slide.Title)
	}
	writeData(writer, request, http.StatusOK, sharedDeck{
		Title: presentation.Title, SlideCount: len(presentation.Slides),
		Titles: titles, Language: presentation.Language,
	})
}

func (s *Server) sharedPreview(writer http.ResponseWriter, request *http.Request) {
	presentation, err := s.presentationFromShare(writer, request)
	if err != nil {
		return
	}
	if len(presentation.Slides) == 0 {
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
	if position > len(presentation.Slides) {
		position = len(presentation.Slides)
	}
	options := pptx.PreviewOptions{Width: previewWidth(request), Media: templateMedia(data)}
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
