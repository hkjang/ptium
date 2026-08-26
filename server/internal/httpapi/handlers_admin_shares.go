package httpapi

import (
	"net/http"
	"strings"

	"github.com/hkjang/ptium/server/internal/store"
)

// What is open, and shutting one thing down.
//
// A share link is the one thing here that reaches somebody with no account, and
// only the deck's owner could see their own. An operator asked "what of ours is
// readable outside?" had no way to answer, and no way to close a link somebody
// had left open — the person who made it might have left the company.

func (s *Server) adminListShares(writer http.ResponseWriter, request *http.Request) {
	limit, offset := pagination(request)
	state := strings.ToLower(strings.TrimSpace(request.URL.Query().Get("state")))
	switch state {
	case "", "open", "expired", "revoked":
	default:
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error",
			"state must be open, expired or revoked", nil)
		return
	}
	shares, total, err := s.store.ListAllShares(request.Context(),
		store.SharesFilter{State: state, Search: searchTerm(request)}, limit, offset)
	if err != nil {
		s.internalError(writer, request, "admin_shares_read_failed", err)
		return
	}
	writeList(writer, request, shares, total, limit, offset)
}

func (s *Server) adminCloseShare(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	closed, didClose, err := s.store.CloseShare(request.Context(), request.PathValue("id"))
	if err != nil {
		s.handleStoreError(writer, request, err, "admin_share_close_failed")
		return
	}
	if didClose {
		// Closing somebody else's link is worth writing down: whose deck it was
		// and who closed it.
		s.store.Audit(request.Context(), &user.ID, "share.close", "share", closed.ID,
			map[string]any{"presentationId": closed.PresentationID, "ownerId": closed.OwnerID,
				"label": closed.Label, "views": closed.Views})
	}
	writeData(writer, request, http.StatusOK, map[string]any{"share": closed, "closed": didClose})
}
