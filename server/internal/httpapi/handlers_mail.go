package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hkjang/ptium/server/internal/mail"
)

// Notification mail: what left the building, and one sent on purpose to prove
// the relay works. Sending on an event happens where the event does — the
// generation worker, the shared-comment handler — through notify, which never
// waits on a relay and never fails the request.

// notify sends one event mail when a mailer is configured.
func (s *Server) notify(ctx context.Context, notification mail.Notification, actorID string, recipients []string) {
	if s.mail == nil || len(recipients) == 0 {
		return
	}
	s.mail.Notify(ctx, notification, actorID, recipients)
}

func (s *Server) adminMailDeliveries(writer http.ResponseWriter, request *http.Request) {
	if s.mail == nil {
		writeData(writer, request, http.StatusOK, mail.Page{Items: []mail.Delivery{}, Status: map[string]int{}})
		return
	}
	limit, _ := strconv.Atoi(request.URL.Query().Get("limit"))
	page, err := s.mail.Deliveries(request.Context(), request.URL.Query().Get("status"), limit)
	if err != nil {
		s.handleStoreError(writer, request, err, "mail_deliveries_failed")
		return
	}
	writeData(writer, request, http.StatusOK, page)
}

// adminSendTestMail sends one mail with the settings as saved and says how it
// went, right there: a relay is rarely described correctly the first time.
func (s *Server) adminSendTestMail(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Recipient string `json:"recipient"`
	}
	if !decodeJSON(writer, request, &input) {
		return
	}
	user, _ := UserFromContext(request.Context())
	recipient := strings.TrimSpace(input.Recipient)
	if recipient == "" {
		recipient = strings.TrimSpace(user.Email)
	}
	if !strings.Contains(recipient, "@") || strings.ContainsAny(recipient, " <>,") {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", "recipient must be one email address", nil)
		return
	}
	if s.mail == nil {
		writeError(writer, request, http.StatusServiceUnavailable, "mail_unavailable", "mail is not configured on this server", nil)
		return
	}
	err := s.mail.SendNow(request.Context(), mail.TestMessage(time.Now()), user.ID, recipient)
	switch {
	case errors.Is(err, mail.ErrDisabled):
		writeError(writer, request, http.StatusConflict, "mail_disabled", "mail is switched off; save mail.enabled first", nil)
	case errors.Is(err, mail.ErrInvalid):
		writeError(writer, request, http.StatusUnprocessableEntity, "mail_invalid", err.Error(), nil)
	case err != nil:
		writeError(writer, request, http.StatusBadGateway, "mail_send_failed", err.Error(), nil)
	default:
		writeData(writer, request, http.StatusOK, map[string]any{"sent": true, "recipient": recipient})
	}
}
