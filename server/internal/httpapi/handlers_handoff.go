package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/hkjang/ptium/server/internal/docs"
	"github.com/hkjang/ptium/server/internal/export"
	"github.com/hkjang/ptium/server/internal/handoff"
	"github.com/hkjang/ptium/server/internal/store"
)

// Passing a deck to another service, and taking a document from one, without
// anybody downloading a file (HANDOFF-STANDARD).
//
// A thought starts on a canvas, becomes a document, becomes these slides and
// goes into a weekly report. The formats already matched; what was missing
// was the hand that passes the file on. The shape is shared by every service
// on the network and is not this repository's to vary: the sender issues a
// five-minute, single-use claim bound to one document, and the receiver —
// given the sender's origin and the claim — fetches the document from the
// sender itself. No service holds another's credentials.

// peersFrom is the administrator's allow list as stored now. It is read on
// every request, so a service added in the settings screen is a destination
// on the next click and a service removed stops being accepted at once.
func peersFrom(ctx context.Context, reader interface {
	Get(ctx context.Context, key string, target any) error
}) handoff.Config {
	var entries []string
	if reader == nil || reader.Get(ctx, handoff.SettingKey, &entries) != nil {
		return handoff.Config{}
	}
	config, err := handoff.ParsePeers(entries)
	if err != nil {
		// A stored list that no longer parses is nobody's destination; the
		// settings screen refuses to save one, so this is a hand-edited row.
		return handoff.Config{}
	}
	return config
}

// publicOrigin is where a peer reaches this service: the deployment's public
// address when the operator gave one, and otherwise what the request came in
// as, through whatever proxy is in front.
func (s *Server) publicOrigin(request *http.Request) string {
	if origin := handoff.OriginOf(s.publicBaseURL); origin != "" {
		return origin
	}
	host := strings.TrimSpace(request.Header.Get("X-Forwarded-Host"))
	if first, _, found := strings.Cut(host, ","); found {
		host = strings.TrimSpace(first)
	}
	if host == "" {
		host = request.Host
	}
	scheme := "http"
	if secureRequest(request) {
		scheme = "https"
	}
	return handoff.OriginOf(scheme + "://" + host)
}

// handoffTargets is GET /api/v1/handoff/targets: where a deck can be sent.
// Empty — the list is empty, or nothing on it receives a presentation — is
// what hides the button, so a fresh install shows nothing.
func (s *Server) handoffTargets(writer http.ResponseWriter, request *http.Request) {
	writeData(writer, request, http.StatusOK, map[string]any{
		"source":  s.publicOrigin(request),
		"targets": s.readPeers(request.Context()).Targets(),
	})
}

type handoffClaimRequest struct {
	Resource string `json:"resource"`
	Format   string `json:"format"`
}

// issueHandoffClaim is POST /api/v1/handoff/claims. The claim is bound to the
// one deck named, which the caller must be able to read — the same check as
// exporting it — and the file is built now, under that person's permissions,
// because whoever comes for it brings none. The answer is the standard's,
// unwrapped: it is what every receiving service is written against.
func (s *Server) issueHandoffClaim(writer http.ResponseWriter, request *http.Request) {
	var input handoffClaimRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	format, ok := handoff.NormalizeFormat(input.Format)
	if !ok {
		writeError(writer, request, http.StatusUnprocessableEntity, "unsupported_handoff_format",
			"This service passes a deck on as pptx only", map[string]any{"sends": handoff.Sends})
		return
	}
	user, _ := UserFromContext(request.Context())
	presentation, err := s.store.GetPresentation(request.Context(), strings.TrimSpace(input.Resource), user.ID, false)
	if err != nil {
		s.handleStoreError(writer, request, err, "handoff_claim_failed")
		return
	}
	if len(presentation.Slides) == 0 {
		writeError(writer, request, http.StatusConflict, "presentation_has_no_slides", "Generate or add slides before sending", nil)
		return
	}
	templateData, manifest, err := s.presentationTemplate(request.Context(), presentation)
	if err != nil {
		s.handleStoreError(writer, request, err, "presentation_template_unavailable")
		return
	}
	author := user.Name
	if profile, profileErr := s.store.GetProfile(request.Context(), user.ID); profileErr == nil && strings.TrimSpace(profile.Company) != "" {
		author = strings.TrimSpace(profile.Company)
	}
	options := export.Options{TemplateData: templateData, Manifest: manifest,
		Author: author, Images: s.imageSource(request, user.ID)}
	// Building the package is what an export costs, and it goes through the
	// same gate: the file is the same file.
	release, allowed := s.holdBudget(writer, request, costOfPPTX, printWait, "printing_busy",
		"This deployment is already building as many documents as it can at once. Try again in a moment.")
	if !allowed {
		return
	}
	data, err := export.PPTX(presentation, options)
	release()
	if err != nil {
		s.internalError(writer, request, "handoff_claim_failed", err)
		return
	}
	claim, err := handoff.NewClaim()
	if err != nil {
		s.internalError(writer, request, "handoff_claim_failed", err)
		return
	}
	filename := safeFilename(presentation.Title) + ".pptx"
	expires := time.Now().Add(handoff.ClaimTTL)
	if err := s.store.IssueHandoffClaim(request.Context(), claim, store.HandoffClaim{
		PresentationID: presentation.ID, OwnerID: user.ID, Format: format, Filename: filename,
		ContentType: handoff.MediaType(format), Body: data, ExpiresAt: expires,
	}); err != nil {
		s.internalError(writer, request, "handoff_claim_failed", err)
		return
	}
	// The trail says a claim was issued and for what; it never holds the claim.
	s.store.Audit(request.Context(), &user.ID, "presentation.handoff_offer", "presentation", presentation.ID,
		map[string]any{"format": format, "bytes": len(data)})
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(writer).Encode(map[string]any{
		"claim":        claim,
		"source":       s.publicOrigin(request),
		"filename":     filename,
		"content_type": handoff.MediaType(format),
		"bytes":        len(data),
		"expires_at":   expires.Format(time.RFC3339),
	})
}

// serveHandoffClaim is GET /api/v1/handoff/claims/{claim}, answered without
// a login: the claim is the credential. That is why it is short, single-use
// and bound to one file — and why a claim that was used, has expired, or
// never existed gets the same 404, with no word on which.
func (s *Server) serveHandoffClaim(writer http.ResponseWriter, request *http.Request) {
	claim := request.PathValue("claim")
	if !handoff.ValidClaim(claim) {
		writeError(writer, request, http.StatusNotFound, "not_found", "The requested resource was not found", nil)
		return
	}
	redeemed, err := s.store.RedeemHandoffClaim(request.Context(), claim)
	if errors.Is(err, store.ErrNotFound) {
		writeError(writer, request, http.StatusNotFound, "not_found", "The requested resource was not found", nil)
		return
	}
	if err != nil {
		s.internalError(writer, request, "handoff_serve_failed", err)
		return
	}
	s.store.Audit(request.Context(), nil, "presentation.handoff_served", "presentation", redeemed.PresentationID,
		map[string]any{"format": redeemed.Format, "bytes": len(redeemed.Body)})
	writer.Header().Set("Content-Type", redeemed.ContentType)
	writer.Header().Set("Content-Disposition", handoff.Disposition(redeemed.Filename))
	writer.Header().Set("Content-Length", strconv.Itoa(len(redeemed.Body)))
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(redeemed.Body)
}

type handoffReceiveRequest struct {
	Source     string `json:"source"`
	Claim      string `json:"claim"`
	TemplateID string `json:"templateId,omitempty"`
}

// receiveHandoff is what the workspace's /handoff page calls with the source
// and claim a browser brought. The source came in from outside: fetched as
// it came, this service would read any address on the network on request.
// So only an origin on the administrator's list is asked, and one that is
// not costs no request at all. The document is then read exactly as an
// upload of the same file would be, and the new deck says where it came from.
func (s *Server) receiveHandoff(writer http.ResponseWriter, request *http.Request) {
	var input handoffReceiveRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	user, _ := UserFromContext(request.Context())
	config := s.readPeers(request.Context())
	peer, allowed := config.Allowed(input.Source)
	if !allowed {
		writeError(writer, request, http.StatusForbidden, "handoff_source_not_allowed",
			"That service is not on this deployment's list of services documents are taken from", nil)
		return
	}
	// Reading a document and compiling a deck out of it is what an import
	// costs, and it is held to the same budget.
	release, ok := s.holdBudget(writer, request, costOfImport, templateReadWait, "import_busy",
		"This deployment is already reading as much as it can hold at once. Try again in a moment.")
	if !ok {
		return
	}
	defer release()
	document, err := handoff.Fetch(request.Context(), s.handoffClient, config, input.Source, input.Claim)
	if err != nil {
		s.refuseHandoff(writer, request, err)
		return
	}
	if int64(len(document.Body)) > s.maximumTemplateBytes(request.Context()) {
		writeError(writer, request, http.StatusRequestEntityTooLarge, "handoff_too_large",
			"The document is larger than this deployment reads", nil)
		return
	}
	read, err := docs.Read(document.Filename, document.Body)
	if err != nil {
		writeError(writer, request, http.StatusUnprocessableEntity, "unreadable_document", err.Error(),
			map[string]any{"filename": document.Filename})
		return
	}
	title := firstNonEmpty(read.Title, strings.TrimSuffix(document.Filename, "."+document.Format), "넘겨받은 문서")
	// Where it came from travels with the deck: in the note the person sees
	// when it lands, in the brief the deck keeps, and in the audit trail.
	warnings := append([]string{}, read.Warnings...)
	warnings = append(warnings, fmt.Sprintf("%s(%s)에서 넘겨받은 %s의 내용을 슬라이드로 옮기고 각 장에 출처를 달았습니다",
		peer.Name, peer.Origin, document.Filename))
	meta := templateMetadata{Filename: document.Filename}
	// storeImportedSource reads the design from the form an upload carries;
	// this request carried JSON, so the choice is put where it will be read.
	request.Form = url.Values{"templateId": {strings.TrimSpace(input.TemplateID)}}
	s.store.Audit(request.Context(), &user.ID, "presentation.handoff_receive", "user", user.ID,
		map[string]any{"source": peer.Origin, "peer": peer.Name, "filename": document.Filename, "format": document.Format, "bytes": len(document.Body)})
	s.storeImportedSource(writer, request, user, meta, title,
		fmt.Sprintf("%s(%s)에서 넘겨받은 %s", peer.Name, peer.Origin, document.Filename), read.Source, warnings)
}

// refuseHandoff says, in the answer, why the document did not come — the
// page turns each into a sentence a person can act on.
func (s *Server) refuseHandoff(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, handoff.ErrNotAllowed):
		writeError(writer, request, http.StatusForbidden, "handoff_source_not_allowed",
			"That service is not on this deployment's list of services documents are taken from", nil)
	case errors.Is(err, handoff.ErrBadClaim):
		writeError(writer, request, http.StatusBadRequest, "handoff_claim_invalid", "The claim is not one a service would issue", nil)
	case errors.Is(err, handoff.ErrNotFound):
		writeError(writer, request, http.StatusNotFound, "handoff_claim_refused",
			"The source did not honour the claim: it was already used or has expired", nil)
	case errors.Is(err, handoff.ErrRedirect):
		writeError(writer, request, http.StatusBadGateway, "handoff_redirected",
			"The source answered with a redirect, which is not followed", nil)
	case errors.Is(err, handoff.ErrUnsupported):
		writeError(writer, request, http.StatusUnsupportedMediaType, "handoff_unsupported_format",
			"The source sent a format this service does not read", map[string]any{"receives": handoff.Receives})
	case errors.Is(err, handoff.ErrTooLarge):
		writeError(writer, request, http.StatusRequestEntityTooLarge, "handoff_too_large",
			fmt.Sprintf("The document is larger than %d MB", handoff.MaxBodyBytes>>20), nil)
	default:
		s.logger.Warn("a handoff source could not be read", "request_id", RequestID(request.Context()), "error", err)
		writeError(writer, request, http.StatusBadGateway, "handoff_source_unreachable",
			"The source could not be reached or did not answer in time", nil)
	}
}

// loggedPath is the request path as the log and the error centre see it. A
// served claim is a credential in a path segment, and a log line that kept it
// would hold, for as long as logs are kept, what was meant to live five
// minutes.
func loggedPath(path string) string {
	if rest, found := strings.CutPrefix(path, handoff.ClaimsPath+"/"); found && rest != "" {
		return handoff.ClaimsPath + "/{claim}"
	}
	return path
}
