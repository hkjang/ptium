package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hkjang/ptium/server/internal/model"
)

const resourcePageSize = 50

// ServiceErrorCode identifies an expected, client-safe application error.
type ServiceErrorCode string

const (
	ServiceErrorInvalidArgument ServiceErrorCode = "invalid_argument"
	ServiceErrorNotFound        ServiceErrorCode = "not_found"
	ServiceErrorForbidden       ServiceErrorCode = "forbidden"
	ServiceErrorConflict        ServiceErrorCode = "conflict"
)

// ServiceError lets an Operations implementation return a safe error to an MCP
// client. All other errors are reported through Config.OnError and redacted.
type ServiceError struct {
	Code    ServiceErrorCode
	Message string
}

func (e *ServiceError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// NewServiceError constructs an expected application error safe for clients.
func NewServiceError(code ServiceErrorCode, message string) *ServiceError {
	return &ServiceError{Code: code, Message: message}
}

func (h *Handler) dispatch(ctx context.Context, user model.User, request rpcRequest) (any, *rpcError) {
	switch request.Method {
	case "initialize":
		return h.initialize(request.Params)
	case "notifications/initialized", "notifications/cancelled":
		return map[string]any{}, nil
	case "ping":
		if err := requireEmptyParams(request.Params); err != nil {
			return nil, invalidParams(err.Error())
		}
		return map[string]any{}, nil
	case "tools/list":
		if err := validateListToolsParams(request.Params); err != nil {
			return nil, invalidParams(err.Error())
		}
		return map[string]any{"tools": toolDefinitions()}, nil
	case "tools/call":
		return h.callTool(ctx, user, request.Params)
	case "resources/list":
		return h.listResources(ctx, user, request.Params)
	case "resources/read":
		return h.readResource(ctx, user, request.Params)
	default:
		return nil, &rpcError{Code: -32601, Message: "Method not found"}
	}
}

func (h *Handler) initialize(params json.RawMessage) (any, *rpcError) {
	var input struct {
		ProtocolVersion string          `json:"protocolVersion"`
		Capabilities    json.RawMessage `json:"capabilities"`
		ClientInfo      json.RawMessage `json:"clientInfo"`
		Meta            json.RawMessage `json:"_meta,omitempty"`
	}
	if err := decodeParams(params, &input, false); err != nil || strings.TrimSpace(input.ProtocolVersion) == "" {
		return nil, invalidParams("protocolVersion is required")
	}
	selected := input.ProtocolVersion
	if _, ok := supportedProtocolVersions[selected]; !ok {
		selected = LatestProtocolVersion
	}
	return map[string]any{
		"protocolVersion": selected,
		"capabilities": map[string]any{
			"tools":     map[string]bool{"listChanged": false},
			"resources": map[string]bool{"subscribe": false, "listChanged": false},
		},
		"serverInfo": map[string]string{
			"name":    h.serverName,
			"title":   "Ptium AI Presentations",
			"version": h.serverVersion,
		},
		"instructions": "Use Ptium tools to create, generate, list, and read presentations available to the authenticated user.",
	}, nil
}

type toolDefinition struct {
	Name        string         `json:"name"`
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func toolDefinitions() []toolDefinition {
	return []toolDefinition{
		{
			Name:        "ptium.list_presentations",
			Title:       "List Ptium presentations",
			Description: "List presentations visible to the authenticated Ptium user.",
			InputSchema: objectSchema(map[string]any{
				"limit":  map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 20},
				"offset": map[string]any{"type": "integer", "minimum": 0, "default": 0},
			}, nil),
		},
		{
			Name:        "ptium.get_presentation",
			Title:       "Get a Ptium presentation",
			Description: "Read one presentation, including its generated slides.",
			InputSchema: objectSchema(map[string]any{
				"id": map[string]any{"type": "string", "minLength": 1, "maxLength": 128},
			}, []string{"id"}),
		},
		{
			Name:        "ptium.create_presentation",
			Title:       "Create a Ptium presentation",
			Description: "Create a draft presentation from a title and generation prompt.",
			InputSchema: objectSchema(map[string]any{
				"title":      map[string]any{"type": "string", "minLength": 1, "maxLength": 200},
				"prompt":     map[string]any{"type": "string", "minLength": 1, "maxLength": 20000},
				"theme":      map[string]any{"type": "string", "maxLength": 64},
				"templateId": map[string]any{"type": "string", "maxLength": 128, "description": "PowerPoint template to generate into; defaults to the design matching the theme"},
				"language":   map[string]any{"type": "string", "maxLength": 32},
				"audience":   map[string]any{"type": "string", "maxLength": 200},
				"tone":       map[string]any{"type": "string", "maxLength": 64},
				"slideCount": map[string]any{"type": "integer", "minimum": 1, "maximum": 50, "description": "Defaults to the administrator-configured slide count when omitted"},
			}, []string{"title", "prompt"}),
		},
		{
			Name:        "ptium.generate_presentation",
			Title:       "Generate a Ptium presentation",
			Description: "Queue generation (or regeneration) for an existing presentation.",
			InputSchema: objectSchema(map[string]any{
				"id": map[string]any{"type": "string", "minLength": 1, "maxLength": 128},
			}, []string{"id"}),
		},
		{
			Name:        "ptium.list_templates",
			Title:       "List Ptium presentation templates",
			Description: "List the PowerPoint templates available to the user, with their layouts, so a deck can be created against a specific design.",
			InputSchema: objectSchema(map[string]any{
				"limit":  map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 20},
				"offset": map[string]any{"type": "integer", "minimum": 0, "default": 0},
			}, nil),
		},
	}
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	Meta      json.RawMessage `json:"_meta,omitempty"`
}

type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolResult struct {
	Content           []toolContent `json:"content"`
	StructuredContent any           `json:"structuredContent,omitempty"`
	IsError           bool          `json:"isError,omitempty"`
}

func (h *Handler) callTool(ctx context.Context, user model.User, params json.RawMessage) (any, *rpcError) {
	var call toolCallParams
	if err := decodeParams(params, &call, true); err != nil || strings.TrimSpace(call.Name) == "" {
		return nil, invalidParams("tool name is required")
	}
	if len(call.Arguments) == 0 {
		call.Arguments = json.RawMessage(`{}`)
	}
	if firstNonSpace(call.Arguments) != '{' {
		return nil, invalidParams("tool arguments must be an object")
	}

	var result any
	var err error
	switch call.Name {
	case "ptium.list_presentations":
		var input struct {
			Limit  int `json:"limit,omitempty"`
			Offset int `json:"offset,omitempty"`
		}
		if decodeErr := decodeArguments(call.Arguments, &input); decodeErr != nil {
			return toolFailure(ServiceErrorInvalidArgument, decodeErr.Error()), nil
		}
		if input.Limit == 0 {
			input.Limit = 20
		}
		if input.Limit < 1 || input.Limit > 100 || input.Offset < 0 || input.Offset > 1_000_000 {
			return toolFailure(ServiceErrorInvalidArgument, "limit must be 1-100 and offset must be 0-1000000"), nil
		}
		presentations, total, operationErr := h.operations.ListPresentations(ctx, user, input.Limit, input.Offset)
		err = operationErr
		result = map[string]any{"presentations": nonNilPresentations(presentations), "total": total, "limit": input.Limit, "offset": input.Offset}

	case "ptium.list_templates":
		var input struct {
			Limit  int `json:"limit,omitempty"`
			Offset int `json:"offset,omitempty"`
		}
		if decodeErr := decodeArguments(call.Arguments, &input); decodeErr != nil {
			return toolFailure(ServiceErrorInvalidArgument, decodeErr.Error()), nil
		}
		if input.Limit == 0 {
			input.Limit = 20
		}
		if input.Limit < 1 || input.Limit > 100 || input.Offset < 0 || input.Offset > 1_000_000 {
			return toolFailure(ServiceErrorInvalidArgument, "limit must be 1-100 and offset must be 0-1000000"), nil
		}
		templates, total, operationErr := h.operations.ListTemplates(ctx, user, input.Limit, input.Offset)
		err = operationErr
		result = map[string]any{"templates": nonNilTemplates(templates), "total": total, "limit": input.Limit, "offset": input.Offset}

	case "ptium.get_presentation":
		id, decodeErr := decodeID(call.Arguments)
		if decodeErr != nil {
			return toolFailure(ServiceErrorInvalidArgument, decodeErr.Error()), nil
		}
		presentation, operationErr := h.operations.GetPresentation(ctx, user, id)
		err = operationErr
		result = map[string]any{"presentation": presentation}

	case "ptium.create_presentation":
		var input CreatePresentationInput
		if decodeErr := decodeArguments(call.Arguments, &input); decodeErr != nil {
			return toolFailure(ServiceErrorInvalidArgument, decodeErr.Error()), nil
		}
		if validationErr := normalizeCreateInput(&input); validationErr != nil {
			return toolFailure(ServiceErrorInvalidArgument, validationErr.Error()), nil
		}
		presentation, operationErr := h.operations.CreatePresentation(ctx, user, input)
		err = operationErr
		result = map[string]any{"presentation": presentation}

	case "ptium.generate_presentation":
		id, decodeErr := decodeID(call.Arguments)
		if decodeErr != nil {
			return toolFailure(ServiceErrorInvalidArgument, decodeErr.Error()), nil
		}
		presentation, operationErr := h.operations.GeneratePresentation(ctx, user, id)
		err = operationErr
		result = map[string]any{"presentation": presentation}

	default:
		return nil, &rpcError{Code: -32602, Message: "Unknown tool", Data: map[string]any{"name": call.Name}}
	}

	if err != nil {
		return h.toolOperationFailure(ctx, err), nil
	}
	return toolSuccess(result), nil
}

func toolSuccess(value any) toolResult {
	encoded, err := json.Marshal(value)
	if err != nil {
		return toolFailure("internal_error", "The operation result could not be encoded")
	}
	return toolResult{
		Content:           []toolContent{{Type: "text", Text: string(encoded)}},
		StructuredContent: value,
	}
}

func toolFailure(code ServiceErrorCode, message string) toolResult {
	if strings.TrimSpace(message) == "" {
		message = defaultServiceMessage(code)
	}
	structured := map[string]any{"error": map[string]any{"code": string(code), "message": message}}
	encoded, _ := json.Marshal(structured)
	return toolResult{
		Content:           []toolContent{{Type: "text", Text: string(encoded)}},
		StructuredContent: structured,
		IsError:           true,
	}
}

func (h *Handler) toolOperationFailure(ctx context.Context, err error) toolResult {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		h.report(ctx, err)
		return toolFailure("timeout", "The operation timed out")
	}
	var serviceError *ServiceError
	if errors.As(err, &serviceError) {
		return toolFailure(serviceError.Code, safeServiceMessage(serviceError))
	}
	h.report(ctx, err)
	return toolFailure("internal_error", "The operation could not be completed")
}

func (h *Handler) listResources(ctx context.Context, user model.User, params json.RawMessage) (any, *rpcError) {
	var input struct {
		Cursor string          `json:"cursor,omitempty"`
		Meta   json.RawMessage `json:"_meta,omitempty"`
	}
	if err := decodeParams(params, &input, true); err != nil {
		return nil, invalidParams(err.Error())
	}
	offset, err := decodeCursor(input.Cursor)
	if err != nil {
		return nil, invalidParams("cursor is invalid")
	}
	presentations, total, err := h.operations.ListPresentations(ctx, user, resourcePageSize, offset)
	if err != nil {
		return nil, h.operationRPCError(ctx, err)
	}
	resources := make([]map[string]any, 0, len(presentations))
	for _, presentation := range presentations {
		name := "presentation-" + presentation.ID
		if presentation.ID == "" {
			name = presentation.Title
		}
		resource := map[string]any{
			"uri":         presentationURI(presentation.ID),
			"name":        name,
			"title":       presentation.Title,
			"description": fmt.Sprintf("Ptium presentation (%s)", presentation.Status),
			"mimeType":    "application/json",
		}
		if !presentation.UpdatedAt.IsZero() {
			resource["annotations"] = map[string]any{
				"audience":     []string{"user", "assistant"},
				"lastModified": presentation.UpdatedAt.UTC().Format(time.RFC3339),
			}
		}
		resources = append(resources, resource)
	}
	result := map[string]any{"resources": resources}
	if nextOffset := offset + len(presentations); nextOffset < total {
		result["nextCursor"] = encodeCursor(nextOffset)
	}
	return result, nil
}

func (h *Handler) readResource(ctx context.Context, user model.User, params json.RawMessage) (any, *rpcError) {
	var input struct {
		URI  string          `json:"uri"`
		Meta json.RawMessage `json:"_meta,omitempty"`
	}
	if err := decodeParams(params, &input, true); err != nil {
		return nil, invalidParams(err.Error())
	}
	id, err := presentationIDFromURI(input.URI)
	if err != nil {
		return nil, invalidParams(err.Error())
	}
	presentation, err := h.operations.GetPresentation(ctx, user, id)
	if err != nil {
		return nil, h.operationRPCError(ctx, err)
	}
	encoded, err := json.Marshal(presentation)
	if err != nil {
		h.report(ctx, err)
		return nil, &rpcError{Code: -32603, Message: "Internal error"}
	}
	return map[string]any{
		"contents": []map[string]any{{
			"uri":      presentationURI(presentation.ID),
			"mimeType": "application/json",
			"text":     string(encoded),
		}},
	}, nil
}

func (h *Handler) operationRPCError(ctx context.Context, err error) *rpcError {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		h.report(ctx, err)
		return &rpcError{Code: -32000, Message: "Operation timed out"}
	}
	var serviceError *ServiceError
	if !errors.As(err, &serviceError) {
		h.report(ctx, err)
		return &rpcError{Code: -32603, Message: "Internal error"}
	}
	message := safeServiceMessage(serviceError)
	switch serviceError.Code {
	case ServiceErrorInvalidArgument:
		return &rpcError{Code: -32602, Message: message}
	case ServiceErrorNotFound:
		return &rpcError{Code: -32002, Message: message}
	case ServiceErrorForbidden:
		return &rpcError{Code: -32003, Message: message}
	case ServiceErrorConflict:
		return &rpcError{Code: -32009, Message: message}
	default:
		return &rpcError{Code: -32000, Message: message}
	}
}

func invalidParams(message string) *rpcError {
	if strings.TrimSpace(message) == "" {
		message = "Invalid params"
	}
	return &rpcError{Code: -32602, Message: message}
}

func decodeParams(raw json.RawMessage, target any, strict bool) error {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	if firstNonSpace(raw) != '{' {
		return errors.New("params must be an object")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if strict {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(target); err != nil {
		return errors.New("params are invalid")
	}
	return nil
}

func decodeArguments(raw json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("tool arguments do not match the input schema")
	}
	return nil
}

func requireEmptyParams(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var params map[string]json.RawMessage
	if err := json.Unmarshal(raw, &params); err != nil {
		return errors.New("params must be an object")
	}
	for key := range params {
		if key != "_meta" {
			return errors.New("ping does not accept params")
		}
	}
	return nil
}

func validateListToolsParams(raw json.RawMessage) error {
	var input struct {
		Cursor string          `json:"cursor,omitempty"`
		Meta   json.RawMessage `json:"_meta,omitempty"`
	}
	if err := decodeParams(raw, &input, true); err != nil {
		return err
	}
	if input.Cursor != "" {
		return errors.New("tools list has no additional pages")
	}
	return nil
}

func decodeID(raw json.RawMessage) (string, error) {
	var input struct {
		ID string `json:"id"`
	}
	if err := decodeArguments(raw, &input); err != nil {
		return "", err
	}
	id := strings.TrimSpace(input.ID)
	if id == "" || utf8.RuneCountInString(id) > 128 {
		return "", errors.New("id must contain 1-128 characters")
	}
	return id, nil
}

func normalizeCreateInput(input *CreatePresentationInput) error {
	input.Title = strings.TrimSpace(input.Title)
	input.Prompt = strings.TrimSpace(input.Prompt)
	input.Theme = strings.TrimSpace(input.Theme)
	input.Language = strings.TrimSpace(input.Language)
	input.Audience = strings.TrimSpace(input.Audience)
	input.Tone = strings.TrimSpace(input.Tone)
	if input.Title == "" || utf8.RuneCountInString(input.Title) > 200 {
		return errors.New("title must contain 1-200 characters")
	}
	if input.Prompt == "" || utf8.RuneCountInString(input.Prompt) > 20_000 {
		return errors.New("prompt must contain 1-20000 characters")
	}
	if utf8.RuneCountInString(input.Theme) > 64 {
		return errors.New("theme must contain at most 64 characters")
	}
	if utf8.RuneCountInString(input.Language) > 32 {
		return errors.New("language must contain at most 32 characters")
	}
	if utf8.RuneCountInString(input.Audience) > 200 {
		return errors.New("audience must contain at most 200 characters")
	}
	if utf8.RuneCountInString(input.Tone) > 64 {
		return errors.New("tone must contain at most 64 characters")
	}
	if input.SlideCount < 0 || input.SlideCount > 50 {
		return errors.New("slideCount must be between 1 and 50")
	}
	return nil
}

func presentationURI(id string) string {
	return "ptium://presentations/" + url.PathEscape(id)
}

func presentationIDFromURI(rawURI string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURI))
	if err != nil || parsed.Scheme != "ptium" || parsed.Host != "presentations" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("uri must identify a Ptium presentation")
	}
	escaped := strings.TrimPrefix(parsed.EscapedPath(), "/")
	if escaped == "" || strings.Contains(escaped, "/") {
		return "", errors.New("uri must identify a Ptium presentation")
	}
	id, err := url.PathUnescape(escaped)
	if err != nil {
		return "", errors.New("uri must identify a Ptium presentation")
	}
	id = strings.TrimSpace(id)
	if id == "" || utf8.RuneCountInString(id) > 128 {
		return "", errors.New("uri must identify a Ptium presentation")
	}
	return id, nil
}

func encodeCursor(offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte("offset:" + strconv.Itoa(offset)))
}

func decodeCursor(cursor string) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil || !strings.HasPrefix(string(decoded), "offset:") {
		return 0, errors.New("invalid cursor")
	}
	offset, err := strconv.Atoi(strings.TrimPrefix(string(decoded), "offset:"))
	if err != nil || offset < 0 || offset > 1_000_000 {
		return 0, errors.New("invalid cursor")
	}
	return offset, nil
}

func safeServiceMessage(err *ServiceError) string {
	message := strings.TrimSpace(err.Message)
	if message == "" {
		message = defaultServiceMessage(err.Code)
	}
	if utf8.RuneCountInString(message) > 500 {
		message = string([]rune(message)[:500])
	}
	return message
}

func defaultServiceMessage(code ServiceErrorCode) string {
	switch code {
	case ServiceErrorInvalidArgument:
		return "The request is invalid"
	case ServiceErrorNotFound:
		return "Presentation not found"
	case ServiceErrorForbidden:
		return "The operation is not permitted"
	case ServiceErrorConflict:
		return "The presentation state conflicts with this operation"
	default:
		return "The operation could not be completed"
	}
}

func nonNilTemplates(templates []model.Template) []model.Template {
	if templates == nil {
		return []model.Template{}
	}
	return templates
}

func nonNilPresentations(presentations []model.Presentation) []model.Presentation {
	if presentations == nil {
		return []model.Presentation{}
	}
	return presentations
}

func firstNonSpace(raw []byte) byte {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return 0
	}
	return trimmed[0]
}
