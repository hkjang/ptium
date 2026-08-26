// Package mcp exposes Ptium presentation operations through the Model Context
// Protocol's JSON-RPC 2.0 Streamable HTTP transport.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hkjang/ptium/server/internal/model"
)

const (
	// LatestProtocolVersion is the newest MCP revision implemented by Handler.
	LatestProtocolVersion = "2025-11-25"

	defaultMaxBodyBytes = int64(1 << 20)
	defaultTimeout      = 30 * time.Second
	maxBatchRequests    = 50
)

var supportedProtocolVersions = map[string]struct{}{
	"2025-03-26": {},
	"2025-06-18": {},
	"2025-11-25": {},
}

// UserFromRequest resolves the already-authenticated Ptium user. The callback
// is also the authorization boundary for MCP access (for example, it can check
// an mcp:use API-key scope). Handler rejects empty and disabled users even when
// the callback returns no error.
type UserFromRequest func(*http.Request) (model.User, error)

// PresentationOperations is the application-service boundary used by MCP. Its
// implementation must enforce ownership and administrator policy for user.
type PresentationOperations interface {
	// The last argument is what the caller is looking for: an agent's first job
	// is finding the right deck, and paging through a thousand is not finding.
	ListPresentations(context.Context, model.User, int, int, string) ([]model.Presentation, int, error)
	GetPresentation(context.Context, model.User, string) (model.Presentation, error)
	CreatePresentation(context.Context, model.User, CreatePresentationInput) (model.Presentation, error)
	GeneratePresentation(context.Context, model.User, string) (model.Presentation, error)
	ListTemplates(context.Context, model.User, int, int) ([]model.Template, int, error)
}

// CreatePresentationInput is the transport-neutral input passed to the
// application service by ptium.create_presentation.
type CreatePresentationInput struct {
	Title      string `json:"title"`
	Prompt     string `json:"prompt"`
	Theme      string `json:"theme,omitempty"`
	TemplateID string `json:"templateId,omitempty"`
	Language   string `json:"language,omitempty"`
	Audience   string `json:"audience,omitempty"`
	Tone       string `json:"tone,omitempty"`
	SlideCount int    `json:"slideCount"`
}

// Config configures a stateless Streamable HTTP handler.
type Config struct {
	Operations       PresentationOperations
	UserFromRequest  UserFromRequest
	ServerName       string
	ServerVersion    string
	MaxBodyBytes     int64
	OperationTimeout time.Duration

	// AllowedOrigins contains additional exact HTTP(S) origins. Requests with no
	// Origin header and same-host origins are always accepted.
	AllowedOrigins []string

	// OnError receives unexpected service errors and recovered panics. It must
	// not expose errors directly to clients and should return quickly.
	OnError func(context.Context, error)
}

// Handler implements http.Handler for the /mcp endpoint. It is stateless and
// safe for concurrent use.
type Handler struct {
	operations       PresentationOperations
	userFromRequest  UserFromRequest
	serverName       string
	serverVersion    string
	maxBodyBytes     int64
	operationTimeout time.Duration
	allowedOrigins   map[string]struct{}
	onError          func(context.Context, error)
}

// New constructs a Ptium MCP handler. Both Operations and UserFromRequest are
// required so the endpoint cannot accidentally be mounted without an
// authentication/authorization boundary.
func New(config Config) (*Handler, error) {
	if config.Operations == nil {
		return nil, errors.New("mcp: presentation operations are required")
	}
	if config.UserFromRequest == nil {
		return nil, errors.New("mcp: user resolver is required")
	}
	if config.MaxBodyBytes < 0 {
		return nil, errors.New("mcp: max body bytes cannot be negative")
	}
	if config.OperationTimeout < 0 {
		return nil, errors.New("mcp: operation timeout cannot be negative")
	}

	serverName := strings.TrimSpace(config.ServerName)
	if serverName == "" {
		serverName = "ptium"
	}
	serverVersion := strings.TrimSpace(config.ServerVersion)
	if serverVersion == "" {
		serverVersion = "dev"
	}
	maxBodyBytes := config.MaxBodyBytes
	if maxBodyBytes == 0 {
		maxBodyBytes = defaultMaxBodyBytes
	}
	operationTimeout := config.OperationTimeout
	if operationTimeout == 0 {
		operationTimeout = defaultTimeout
	}

	allowedOrigins := make(map[string]struct{}, len(config.AllowedOrigins))
	for _, rawOrigin := range config.AllowedOrigins {
		origin, err := normalizeOrigin(rawOrigin)
		if err != nil {
			return nil, fmt.Errorf("mcp: invalid allowed origin %q: %w", rawOrigin, err)
		}
		allowedOrigins[origin] = struct{}{}
	}

	return &Handler{
		operations:       config.Operations,
		userFromRequest:  config.UserFromRequest,
		serverName:       serverName,
		serverVersion:    serverVersion,
		maxBodyBytes:     maxBodyBytes,
		operationTimeout: operationTimeout,
		allowedOrigins:   allowedOrigins,
		onError:          config.OnError,
	}, nil
}

// ServeHTTP handles POST JSON-RPC messages. GET without an SSE-only Accept
// header returns endpoint metadata for humans and diagnostics; an MCP SSE GET
// receives 405 because this stateless implementation does not open server
// event streams.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if recovered := recover(); recovered != nil {
			h.report(r.Context(), fmt.Errorf("panic at MCP transport boundary: %v", recovered))
			h.writeTransportError(w, http.StatusInternalServerError, -32603, "Internal error")
		}
	}()

	h.setCommonHeaders(w)
	if !h.originAllowed(r) {
		h.writeTransportError(w, http.StatusForbidden, -32003, "Origin is not allowed")
		return
	}

	user, err := h.userFromRequest(r)
	if err != nil || strings.TrimSpace(user.ID) == "" {
		h.writeTransportError(w, http.StatusUnauthorized, -32001, "Authentication required")
		return
	}
	if user.Disabled {
		h.writeTransportError(w, http.StatusForbidden, -32003, "User is disabled")
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.handleGet(w, r)
	case http.MethodPost:
		h.handlePost(w, r, user)
	default:
		w.Header().Set("Allow", "GET, POST")
		h.writeTransportError(w, http.StatusMethodNotAllowed, -32600, "Method not allowed")
	}
}

func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	if accepts(r.Header.Get("Accept"), "text/event-stream") {
		w.Header().Set("Allow", "POST")
		h.writeTransportError(w, http.StatusMethodNotAllowed, -32600, "SSE streams are not supported")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{
		"name":              h.serverName,
		"version":           h.serverVersion,
		"transport":         "streamable-http",
		"protocolVersion":   LatestProtocolVersion,
		"authentication":    "required",
		"sessionsSupported": false,
	})
}

func (h *Handler) handlePost(w http.ResponseWriter, r *http.Request, user model.User) {
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		h.writeTransportError(w, http.StatusUnsupportedMediaType, -32600, "Content-Type must be application/json")
		return
	}
	protocolVersion := strings.TrimSpace(r.Header.Get("MCP-Protocol-Version"))
	if protocolVersion == "" {
		protocolVersion = "2025-03-26"
	} else if _, ok := supportedProtocolVersions[protocolVersion]; !ok {
		h.writeTransportError(w, http.StatusBadRequest, -32600, "Unsupported MCP protocol version")
		return
	}

	bodyReader := http.MaxBytesReader(w, r.Body, h.maxBodyBytes)
	body, err := io.ReadAll(bodyReader)
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			h.writeTransportError(w, http.StatusRequestEntityTooLarge, -32600, "Request body is too large")
			return
		}
		h.report(r.Context(), fmt.Errorf("read MCP request: %w", err))
		h.writeTransportError(w, http.StatusBadRequest, -32700, "Parse error")
		return
	}
	if !utf8.Valid(body) || !json.Valid(body) {
		h.writeResponse(w, http.StatusOK, rpcErrorResponse(nullID, -32700, "Parse error", nil))
		return
	}

	trimmed := strings.TrimSpace(string(body))
	if strings.HasPrefix(trimmed, "[") {
		// Batches were part of the 2025-03-26 transport and are retained only
		// for clients using that revision.
		if protocolVersion != "2025-03-26" {
			h.writeTransportError(w, http.StatusBadRequest, -32600, "JSON-RPC batches are not supported by this protocol version")
			return
		}
		h.handleBatch(w, r, user, json.RawMessage(body))
		return
	}

	response, notification := h.processMessage(r.Context(), user, json.RawMessage(body))
	if notification {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	h.writeResponse(w, http.StatusOK, response)
}

func (h *Handler) handleBatch(w http.ResponseWriter, r *http.Request, user model.User, raw json.RawMessage) {
	var messages []json.RawMessage
	if err := json.Unmarshal(raw, &messages); err != nil {
		h.writeResponse(w, http.StatusOK, rpcErrorResponse(nullID, -32600, "Invalid Request", nil))
		return
	}
	if len(messages) == 0 {
		h.writeResponse(w, http.StatusOK, rpcErrorResponse(nullID, -32600, "Invalid Request", nil))
		return
	}
	if len(messages) > maxBatchRequests {
		h.writeTransportError(w, http.StatusBadRequest, -32600, "JSON-RPC batch is too large")
		return
	}

	responses := make([]rpcResponse, 0, len(messages))
	batchContext, cancel := context.WithTimeout(r.Context(), h.operationTimeout)
	defer cancel()
	for _, message := range messages {
		response, notification := h.processMessage(batchContext, user, message)
		if !notification {
			responses = append(responses, response)
		}
	}
	if len(responses) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	h.writeJSON(w, http.StatusOK, responses)
}

func (h *Handler) processMessage(ctx context.Context, user model.User, raw json.RawMessage) (response rpcResponse, notification bool) {
	request, validationError := parseRequest(raw)
	if validationError != nil {
		return rpcErrorResponse(validationError.id, validationError.code, validationError.message, validationError.data), false
	}

	operationContext, cancel := context.WithTimeout(ctx, h.operationTimeout)
	defer cancel()

	defer func() {
		if recovered := recover(); recovered != nil {
			h.report(ctx, fmt.Errorf("panic handling MCP method %q: %v", request.Method, recovered))
			response = rpcErrorResponse(request.ID, -32603, "Internal error", nil)
			notification = !request.HasID
		}
	}()

	result, rpcErr := h.dispatch(operationContext, user, request)
	if !request.HasID {
		return rpcResponse{}, true
	}
	if rpcErr != nil {
		return rpcErrorResponse(request.ID, rpcErr.Code, rpcErr.Message, rpcErr.Data), false
	}
	return rpcResultResponse(request.ID, result), false
}

func (h *Handler) setCommonHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

func (h *Handler) writeTransportError(w http.ResponseWriter, status, code int, message string) {
	h.writeResponse(w, status, rpcErrorResponse(nullID, code, message, nil))
}

func (h *Handler) writeResponse(w http.ResponseWriter, status int, response rpcResponse) {
	h.writeJSON(w, status, response)
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if status == http.StatusNoContent {
		return
	}
	if err := json.NewEncoder(w).Encode(value); err != nil {
		h.report(context.Background(), fmt.Errorf("write MCP response: %w", err))
	}
}

func (h *Handler) report(ctx context.Context, err error) {
	if h.onError == nil || err == nil {
		return
	}
	defer func() { _ = recover() }()
	h.onError(context.WithoutCancel(ctx), err)
}

func (h *Handler) originAllowed(r *http.Request) bool {
	rawOrigin := strings.TrimSpace(r.Header.Get("Origin"))
	if rawOrigin == "" {
		return true
	}
	origin, err := normalizeOrigin(rawOrigin)
	if err != nil {
		return false
	}
	parsed, _ := url.Parse(origin)
	if strings.EqualFold(parsed.Host, r.Host) {
		return true
	}
	_, ok := h.allowedOrigins[origin]
	return ok
}

func normalizeOrigin(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("origin must be an exact http(s) scheme and authority")
	}
	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host), nil
}

func isJSONContentType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return false
	}
	mediaType = strings.ToLower(mediaType)
	return mediaType == "application/json" || (strings.HasPrefix(mediaType, "application/") && strings.HasSuffix(mediaType, "+json"))
}

func accepts(header, mediaType string) bool {
	for _, value := range strings.Split(header, ",") {
		candidate, _, err := mime.ParseMediaType(strings.TrimSpace(value))
		if err == nil && strings.EqualFold(candidate, mediaType) {
			return true
		}
	}
	return false
}
