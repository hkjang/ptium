package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hkjang/ptium/server/internal/model"
)

type fakeOperations struct {
	list func(context.Context, model.User, int, int) ([]model.Presentation, int, error)
	// searched keeps what the last list was asked to look for, so a test can
	// check the tool passes it on rather than dropping it.
	searched  string
	get       func(context.Context, model.User, string) (model.Presentation, error)
	create    func(context.Context, model.User, CreatePresentationInput) (model.Presentation, error)
	generate  func(context.Context, model.User, string) (model.Presentation, error)
	templates func(context.Context, model.User, int, int) ([]model.Template, int, error)
}

func (f *fakeOperations) ListTemplates(ctx context.Context, user model.User, limit, offset int) ([]model.Template, int, error) {
	if f.templates == nil {
		return nil, 0, nil
	}
	return f.templates(ctx, user, limit, offset)
}

func (f *fakeOperations) ListPresentations(ctx context.Context, user model.User, limit, offset int, search string) ([]model.Presentation, int, error) {
	f.searched = search
	if f.list == nil {
		return nil, 0, nil
	}
	return f.list(ctx, user, limit, offset)
}

func (f *fakeOperations) GetPresentation(ctx context.Context, user model.User, id string) (model.Presentation, error) {
	if f.get == nil {
		return model.Presentation{}, nil
	}
	return f.get(ctx, user, id)
}

func (f *fakeOperations) CreatePresentation(ctx context.Context, user model.User, input CreatePresentationInput) (model.Presentation, error) {
	if f.create == nil {
		return model.Presentation{}, nil
	}
	return f.create(ctx, user, input)
}

func (f *fakeOperations) GeneratePresentation(ctx context.Context, user model.User, id string) (model.Presentation, error) {
	if f.generate == nil {
		return model.Presentation{}, nil
	}
	return f.generate(ctx, user, id)
}

func TestNewRequiresDependenciesAndValidConfig(t *testing.T) {
	operations := &fakeOperations{}
	resolver := authenticatedUser
	tests := []struct {
		name   string
		config Config
	}{
		{name: "operations", config: Config{UserFromRequest: resolver}},
		{name: "resolver", config: Config{Operations: operations}},
		{name: "body size", config: Config{Operations: operations, UserFromRequest: resolver, MaxBodyBytes: -1}},
		{name: "timeout", config: Config{Operations: operations, UserFromRequest: resolver, OperationTimeout: -1}},
		{name: "origin", config: Config{Operations: operations, UserFromRequest: resolver, AllowedOrigins: []string{"*"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := New(test.config); err == nil {
				t.Fatal("New() error = nil")
			}
		})
	}

	handler, err := New(Config{Operations: operations, UserFromRequest: resolver})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if handler.serverName != "ptium" || handler.maxBodyBytes != defaultMaxBodyBytes || handler.operationTimeout != defaultTimeout {
		t.Fatalf("defaults = %#v", handler)
	}
}

func TestInitializeAndToolDiscovery(t *testing.T) {
	handler := newTestHandler(t, &fakeOperations{})

	response := postRPC(t, handler, `{"jsonrpc":"2.0","id":"init-1","method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("initialize status = %d, body = %s", response.Code, response.Body.String())
	}
	payload := decodeObject(t, response)
	if payload["id"] != "init-1" {
		t.Fatalf("initialize id = %#v", payload["id"])
	}
	result := payload["result"].(map[string]any)
	if result["protocolVersion"] != LatestProtocolVersion {
		t.Fatalf("protocolVersion = %#v", result["protocolVersion"])
	}
	capabilities := result["capabilities"].(map[string]any)
	if capabilities["tools"] == nil || capabilities["resources"] == nil {
		t.Fatalf("capabilities = %#v", capabilities)
	}

	response = postRPC(t, handler, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`, nil)
	payload = decodeObject(t, response)
	tools := payload["result"].(map[string]any)["tools"].([]any)
	want := []string{
		"ptium.list_presentations",
		"ptium.get_presentation",
		"ptium.create_presentation",
		"ptium.generate_presentation",
		"ptium.list_templates",
	}
	if len(tools) != len(want) {
		t.Fatalf("tool count = %d", len(tools))
	}
	for index, name := range want {
		definition := tools[index].(map[string]any)
		if definition["name"] != name || definition["inputSchema"] == nil {
			t.Fatalf("tool[%d] = %#v", index, definition)
		}
	}
}

func TestToolCallsUseAuthenticatedPresentationOperations(t *testing.T) {
	user := model.User{ID: "user-1", Email: "user@example.com"}
	var calls []string
	operations := &fakeOperations{
		list: func(_ context.Context, gotUser model.User, limit, offset int) ([]model.Presentation, int, error) {
			if gotUser.ID != user.ID || limit != 5 || offset != 2 {
				t.Fatalf("list args = %#v, %d, %d", gotUser, limit, offset)
			}
			calls = append(calls, "list")
			return []model.Presentation{{ID: "deck-list", Title: "Listed"}}, 9, nil
		},
		get: func(_ context.Context, gotUser model.User, id string) (model.Presentation, error) {
			if gotUser.ID != user.ID || id != "deck-get" {
				t.Fatalf("get args = %#v, %q", gotUser, id)
			}
			calls = append(calls, "get")
			return model.Presentation{ID: id, Title: "Read"}, nil
		},
		create: func(_ context.Context, gotUser model.User, input CreatePresentationInput) (model.Presentation, error) {
			if gotUser.ID != user.ID || input.Title != "Quarterly plan" || input.Prompt != "Build the plan" || input.Audience != "Executives" || input.Tone != "Concise" || input.SlideCount != 0 {
				t.Fatalf("create args = %#v, %#v", gotUser, input)
			}
			calls = append(calls, "create")
			return model.Presentation{ID: "deck-created", Title: input.Title, RequestedSlideCount: input.SlideCount}, nil
		},
		generate: func(_ context.Context, gotUser model.User, id string) (model.Presentation, error) {
			if gotUser.ID != user.ID || id != "deck-generate" {
				t.Fatalf("generate args = %#v, %q", gotUser, id)
			}
			calls = append(calls, "generate")
			return model.Presentation{ID: id, Status: "queued"}, nil
		},
	}
	handler, err := New(Config{Operations: operations, UserFromRequest: func(*http.Request) (model.User, error) { return user, nil }})
	if err != nil {
		t.Fatal(err)
	}

	tests := []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ptium.list_presentations","arguments":{"limit":5,"offset":2}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"ptium.get_presentation","arguments":{"id":"deck-get"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"ptium.create_presentation","arguments":{"title":" Quarterly plan ","prompt":" Build the plan ","audience":" Executives ","tone":" Concise "}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"ptium.generate_presentation","arguments":{"id":"deck-generate"}}}`,
	}
	for _, body := range tests {
		response := postRPC(t, handler, body, nil)
		if response.Code != http.StatusOK || strings.Contains(response.Body.String(), `"isError":true`) {
			t.Fatalf("tool response = %d %s", response.Code, response.Body.String())
		}
		payload := decodeObject(t, response)
		result := payload["result"].(map[string]any)
		if result["content"] == nil || result["structuredContent"] == nil {
			t.Fatalf("tool result = %#v", result)
		}
	}
	if strings.Join(calls, ",") != "list,get,create,generate" {
		t.Fatalf("calls = %v", calls)
	}
}

func TestResourcesListPaginationAndRead(t *testing.T) {
	updated := time.Date(2026, time.August, 1, 12, 0, 0, 0, time.UTC)
	var listOffsets []int
	var readID string
	operations := &fakeOperations{
		list: func(_ context.Context, _ model.User, limit, offset int) ([]model.Presentation, int, error) {
			if limit != resourcePageSize {
				t.Fatalf("resource limit = %d", limit)
			}
			listOffsets = append(listOffsets, offset)
			if offset == 0 {
				return []model.Presentation{{ID: "one", Title: "One", Status: "completed", UpdatedAt: updated}, {ID: "two", Title: "Two"}}, 3, nil
			}
			return []model.Presentation{{ID: "three", Title: "Three"}}, 3, nil
		},
		get: func(_ context.Context, _ model.User, id string) (model.Presentation, error) {
			readID = id
			return model.Presentation{ID: id, Title: "One", Slides: []model.Slide{{Title: "Slide"}}}, nil
		},
	}
	handler := newTestHandler(t, operations)

	response := postRPC(t, handler, `{"jsonrpc":"2.0","id":1,"method":"resources/list","params":{}}`, nil)
	result := decodeObject(t, response)["result"].(map[string]any)
	resources := result["resources"].([]any)
	if len(resources) != 2 || resources[0].(map[string]any)["uri"] != "ptium://presentations/one" {
		t.Fatalf("resources = %#v", resources)
	}
	cursor, ok := result["nextCursor"].(string)
	if !ok || cursor == "" {
		t.Fatalf("nextCursor = %#v", result["nextCursor"])
	}

	response = postRPC(t, handler, `{"jsonrpc":"2.0","id":2,"method":"resources/list","params":{"cursor":"`+cursor+`"}}`, nil)
	result = decodeObject(t, response)["result"].(map[string]any)
	if _, ok := result["nextCursor"]; ok {
		t.Fatalf("unexpected next cursor: %#v", result)
	}
	if len(listOffsets) != 2 || listOffsets[1] != 2 {
		t.Fatalf("offsets = %v", listOffsets)
	}

	response = postRPC(t, handler, `{"jsonrpc":"2.0","id":3,"method":"resources/read","params":{"uri":"ptium://presentations/one"}}`, nil)
	result = decodeObject(t, response)["result"].(map[string]any)
	contents := result["contents"].([]any)
	text := contents[0].(map[string]any)["text"].(string)
	if readID != "one" || !strings.Contains(text, `"slides"`) {
		t.Fatalf("read id/text = %q / %s", readID, text)
	}
}

func TestJSONRPCValidationNotificationsAndLegacyBatch(t *testing.T) {
	handler := newTestHandler(t, &fakeOperations{})
	tests := []struct {
		name string
		body string
		code int
	}{
		{name: "parse", body: `{`, code: -32700},
		{name: "non UTF-8", body: string([]byte{'{', '"', 0xff, '"', ':', '1', '}'}), code: -32700},
		{name: "invalid request", body: `{"jsonrpc":"1.0","id":1,"method":"ping"}`, code: -32600},
		{name: "invalid id", body: `{"jsonrpc":"2.0","id":true,"method":"ping"}`, code: -32600},
		{name: "invalid params", body: `{"jsonrpc":"2.0","id":1,"method":"ping","params":"bad"}`, code: -32602},
		{name: "method", body: `{"jsonrpc":"2.0","id":1,"method":"missing"}`, code: -32601},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := postRPC(t, handler, test.body, nil)
			if response.Code != http.StatusOK {
				t.Fatalf("HTTP status = %d", response.Code)
			}
			payload := decodeObject(t, response)
			errorObject := payload["error"].(map[string]any)
			if int(errorObject["code"].(float64)) != test.code {
				t.Fatalf("error = %#v", errorObject)
			}
		})
	}

	notification := postRPC(t, handler, `{"jsonrpc":"2.0","method":"notifications/initialized"}`, nil)
	if notification.Code != http.StatusNoContent || notification.Body.Len() != 0 {
		t.Fatalf("notification = %d %q", notification.Code, notification.Body.String())
	}

	batch := postRPC(t, handler, `[{"jsonrpc":"2.0","id":1,"method":"ping"},{"jsonrpc":"2.0","method":"notifications/initialized"},1]`, nil)
	var batchPayload []map[string]any
	if err := json.Unmarshal(batch.Body.Bytes(), &batchPayload); err != nil {
		t.Fatalf("batch JSON = %v: %s", err, batch.Body.String())
	}
	if len(batchPayload) != 2 {
		t.Fatalf("batch responses = %#v", batchPayload)
	}

	currentBatch := postRPC(t, handler, `[{"jsonrpc":"2.0","id":1,"method":"ping"}]`, func(request *http.Request) {
		request.Header.Set("MCP-Protocol-Version", LatestProtocolVersion)
	})
	if currentBatch.Code != http.StatusBadRequest {
		t.Fatalf("current batch status = %d", currentBatch.Code)
	}
}

func TestTransportAuthenticationOriginAndLimits(t *testing.T) {
	operations := &fakeOperations{}
	unauthenticated, err := New(Config{
		Operations: operations,
		UserFromRequest: func(*http.Request) (model.User, error) {
			return model.User{}, errors.New("no token")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	response := postRPC(t, unauthenticated, `{"jsonrpc":"2.0","id":1,"method":"ping"}`, nil)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", response.Code)
	}

	disabled, err := New(Config{
		Operations: operations,
		UserFromRequest: func(*http.Request) (model.User, error) {
			return model.User{ID: "disabled", Disabled: true}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	response = postRPC(t, disabled, `{"jsonrpc":"2.0","id":1,"method":"ping"}`, nil)
	if response.Code != http.StatusForbidden {
		t.Fatalf("disabled status = %d", response.Code)
	}

	handler, err := New(Config{Operations: operations, UserFromRequest: authenticatedUser, MaxBodyBytes: 64, AllowedOrigins: []string{"https://trusted.example"}})
	if err != nil {
		t.Fatal(err)
	}
	response = postRPC(t, handler, `{"jsonrpc":"2.0","id":1,"method":"ping"}`, func(request *http.Request) {
		request.Header.Set("Origin", "https://evil.example")
	})
	if response.Code != http.StatusForbidden {
		t.Fatalf("origin status = %d", response.Code)
	}
	response = postRPC(t, handler, `{"jsonrpc":"2.0","id":123456789,"method":"ping","params":{"padding":"too large"}}`, nil)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("large body status = %d body=%s", response.Code, response.Body.String())
	}

	request := httptest.NewRequest(http.MethodGet, "http://example.com/mcp", nil)
	request.Header.Set("Accept", "text/event-stream")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("SSE GET status = %d", response.Code)
	}
}

func TestAuthenticationResolverPanicIsRecovered(t *testing.T) {
	var reported error
	handler, err := New(Config{
		Operations: &fakeOperations{},
		UserFromRequest: func(*http.Request) (model.User, error) {
			panic("resolver failed")
		},
		OnError: func(_ context.Context, err error) { reported = err },
	})
	if err != nil {
		t.Fatal(err)
	}
	response := postRPC(t, handler, `{"jsonrpc":"2.0","id":1,"method":"ping"}`, nil)
	if response.Code != http.StatusInternalServerError || reported == nil {
		t.Fatalf("panic response/reported = %d %s / %v", response.Code, response.Body.String(), reported)
	}
	if strings.Contains(response.Body.String(), "resolver failed") {
		t.Fatalf("panic detail leaked: %s", response.Body.String())
	}
}

func TestOperationErrorsAreSafeAndReported(t *testing.T) {
	secret := "postgres://user:password@db/private"
	var reported error
	operations := &fakeOperations{
		get: func(context.Context, model.User, string) (model.Presentation, error) {
			return model.Presentation{}, errors.New(secret)
		},
	}
	handler, err := New(Config{
		Operations:      operations,
		UserFromRequest: authenticatedUser,
		OnError:         func(_ context.Context, err error) { reported = err },
	})
	if err != nil {
		t.Fatal(err)
	}
	response := postRPC(t, handler, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ptium.get_presentation","arguments":{"id":"deck"}}}`, nil)
	if reported == nil || !strings.Contains(reported.Error(), secret) {
		t.Fatalf("reported = %v", reported)
	}
	if strings.Contains(response.Body.String(), secret) || !strings.Contains(response.Body.String(), `"isError":true`) {
		t.Fatalf("unsafe error response = %s", response.Body.String())
	}

	operations.get = func(context.Context, model.User, string) (model.Presentation, error) {
		return model.Presentation{}, NewServiceError(ServiceErrorNotFound, "Deck was not found")
	}
	response = postRPC(t, handler, `{"jsonrpc":"2.0","id":2,"method":"resources/read","params":{"uri":"ptium://presentations/deck"}}`, nil)
	payload := decodeObject(t, response)
	errorObject := payload["error"].(map[string]any)
	if int(errorObject["code"].(float64)) != -32002 || errorObject["message"] != "Deck was not found" {
		t.Fatalf("resource error = %#v", errorObject)
	}
}

func TestOperationTimeout(t *testing.T) {
	operations := &fakeOperations{
		list: func(ctx context.Context, _ model.User, _, _ int) ([]model.Presentation, int, error) {
			<-ctx.Done()
			return nil, 0, ctx.Err()
		},
	}
	handler, err := New(Config{Operations: operations, UserFromRequest: authenticatedUser, OperationTimeout: 5 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	response := postRPC(t, handler, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ptium.list_presentations","arguments":{}}}`, nil)
	if !strings.Contains(response.Body.String(), `"code":"timeout"`) {
		t.Fatalf("timeout response = %s", response.Body.String())
	}
}

func newTestHandler(t *testing.T, operations PresentationOperations) *Handler {
	t.Helper()
	handler, err := New(Config{
		Operations:       operations,
		UserFromRequest:  authenticatedUser,
		ServerName:       "ptium-test",
		ServerVersion:    "1.2.3",
		OperationTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return handler
}

func authenticatedUser(*http.Request) (model.User, error) {
	return model.User{ID: "user-1", Email: "user@example.com"}, nil
}

func postRPC(t *testing.T, handler http.Handler, body string, customize func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "http://example.com/mcp", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	if customize != nil {
		customize(request)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeObject(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("response JSON = %v: %s", err, response.Body.String())
	}
	return payload
}
