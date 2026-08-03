package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hkjang/ptium/server/internal/model"
)

func TestInternalErrorIsCapturedOncePerRequest(t *testing.T) {
	var incidents []model.Incident
	server := &Server{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		captureIncident: func(_ context.Context, incident model.Incident) error {
			incidents = append(incidents, incident)
			return nil
		},
	}
	handler := server.requestMiddleware(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		server.internalError(writer, request, "database_failed", errors.New("database unavailable"))
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/presentations", nil))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", response.Code)
	}
	if len(incidents) != 1 {
		t.Fatalf("incidents = %d, want exactly one: %#v", len(incidents), incidents)
	}
	if incidents[0].Kind != "request" || incidents[0].Message != "database unavailable" || incidents[0].RequestID == "" {
		t.Fatalf("specific request incident was not retained: %#v", incidents[0])
	}
}

func TestMiddlewareCapturesOtherwiseUnrecordedServerFailure(t *testing.T) {
	var incidents []model.Incident
	server := &Server{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		captureIncident: func(_ context.Context, incident model.Incident) error {
			incidents = append(incidents, incident)
			return nil
		},
	}
	handler := server.requestMiddleware(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writeError(writer, request, http.StatusServiceUnavailable, "temporarily_unavailable", "Unavailable", nil)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", response.Code)
	}
	if len(incidents) != 1 || incidents[0].Kind != "http" {
		t.Fatalf("generic server failure was not captured exactly once: %#v", incidents)
	}
}
