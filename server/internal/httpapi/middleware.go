package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hkjang/ptium/server/internal/auth"
	"github.com/hkjang/ptium/server/internal/model"
)

type responseRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

type incidentCaptureTracker struct {
	captured atomic.Bool
}

type incidentCaptureTrackerKey struct{}

func (r *responseRecorder) WriteHeader(status int) {
	if r.status != 0 {
		return
	}
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.WriteHeader(http.StatusOK)
	}
	written, err := r.ResponseWriter.Write(data)
	r.bytes += written
	return written, err
}

func (s *Server) requestMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestID := strings.TrimSpace(request.Header.Get("X-Request-ID"))
		if requestID == "" || len(requestID) > 128 {
			requestID = randomRequestID()
		}
		writer.Header().Set("X-Request-ID", requestID)
		started := time.Now()
		recorder := &responseRecorder{ResponseWriter: writer}
		ctx := withRequestID(request.Context(), requestID)
		ctx = context.WithValue(ctx, incidentCaptureTrackerKey{}, &incidentCaptureTracker{})
		defer func() {
			if recovered := recover(); recovered != nil {
				details, _ := json.Marshal(map[string]any{"panic": fmt.Sprint(recovered), "stack": string(debug.Stack()), "method": request.Method, "path": request.URL.Path})
				s.capture(ctx, model.Incident{RequestID: requestID, Kind: "panic", Severity: "critical", Message: "unhandled server panic", Details: details})
				if recorder.status == 0 {
					writeError(recorder, request.WithContext(ctx), http.StatusInternalServerError, "internal_error", "The server could not complete the request", nil)
				}
			}
			status := recorder.status
			if status == 0 {
				status = http.StatusOK
			}
			s.logger.Info("http request", "request_id", requestID, "method", request.Method, "path", request.URL.Path,
				"status", status, "bytes", recorder.bytes, "duration_ms", time.Since(started).Milliseconds())
			if status >= 500 {
				details, _ := json.Marshal(map[string]any{"method": request.Method, "path": request.URL.Path, "status": status})
				s.capture(ctx, model.Incident{RequestID: requestID, Kind: "http", Severity: "error", Message: fmt.Sprintf("HTTP %d on %s %s", status, request.Method, request.URL.Path), Details: details})
			}
		}()
		next.ServeHTTP(recorder, request.WithContext(ctx))
	})
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	allowed := map[string]struct{}{}
	allowAny := false
	for _, origin := range s.corsOrigins {
		if origin == "*" {
			allowAny = true
		} else {
			allowed[strings.TrimRight(origin, "/")] = struct{}{}
		}
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		origin := strings.TrimRight(request.Header.Get("Origin"), "/")
		if origin != "" {
			_, ok := allowed[origin]
			if allowAny || ok {
				writer.Header().Set("Access-Control-Allow-Origin", origin)
				writer.Header().Set("Access-Control-Allow-Credentials", "true")
				writer.Header().Add("Vary", "Origin")
			}
		}
		if request.Method == http.MethodOptions {
			writer.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
			writer.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type,X-API-Key,X-Ptium-Dev-Secret,X-Request-ID")
			writer.Header().Set("Access-Control-Max-Age", "600")
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

// sessionRenewalMiddleware extends a cookie session that is running out.
//
// Without this, a session ends abruptly at its lifetime no matter how active its
// holder was, which reads as being logged out at random. Renewal is written
// straight back as a cookie, so an idle session still lapses on schedule while
// someone working in the product stays signed in.
func (s *Server) sessionRenewalMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if s.sessions != nil {
			if principal, ok := auth.PrincipalFromContext(request.Context()); ok {
				if state, found := auth.SessionStateFromPrincipal(principal); found && state.FromCookie {
					// Halfway through its life: early enough that a slow request or a
					// clock skew of minutes cannot land after the expiry.
					if time.Until(state.ExpiresAt) < s.sessions.Lifetime()/2 {
						if token, expiresAt, err := s.sessions.Issue(state.UserID, state.Epoch); err == nil {
							setSessionCookie(writer, auth.SessionCookie(token, expiresAt, secureRequest(request)))
						} else {
							s.logger.Warn("could not renew session cookie", "request_id", RequestID(request.Context()), "error", err)
						}
					}
				}
			}
		}
		next.ServeHTTP(writer, request)
	})
}

func (s *Server) identityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		principal, ok := auth.PrincipalFromContext(request.Context())
		if !ok {
			writeError(writer, request, http.StatusUnauthorized, "authentication_required", "Authentication is required", nil)
			return
		}
		var user model.User
		var err error
		if principal.Claims != nil {
			if id, ok := principal.Claims["ptium_user_id"].(string); ok && id != "" {
				user, err = s.store.GetUser(request.Context(), id)
			}
		}
		if user.ID == "" && err == nil {
			admin := principal.HasAnyRole(s.adminRoles...) || containsFold(s.bootstrapAdminEmails, principal.Email) || contains(s.bootstrapAdminSubjects, principal.Subject)
			user, err = s.store.UpsertUser(request.Context(), principal.Subject, principal.Email, principal.Name, principal.Roles, admin)
		}
		if err != nil {
			s.internalError(writer, request, "identity_provision_failed", err)
			return
		}
		if user.Disabled {
			writeError(writer, request, http.StatusForbidden, "account_disabled", "This account has been disabled", nil)
			return
		}
		next.ServeHTTP(writer, request.WithContext(withUser(request.Context(), user)))
	})
}

func (s *Server) requireAdmin(scope string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		user, ok := UserFromContext(request.Context())
		if !ok || !user.IsAdmin {
			writeError(writer, request, http.StatusForbidden, "admin_required", "Administrator access is required", nil)
			return
		}
		if !allowScope(request.Context(), scope) {
			writeError(writer, request, http.StatusForbidden, "insufficient_scope", "The API key does not grant this operation", map[string]any{"required": scope})
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func requireScope(scope string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !allowScope(request.Context(), scope) {
			writeError(writer, request, http.StatusForbidden, "insufficient_scope", "The API key does not grant this operation", map[string]any{"required": scope})
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func allowScope(ctx context.Context, scope string) bool {
	principal, ok := auth.PrincipalFromContext(ctx)
	if !ok {
		return false
	}
	return principal.AuthMethod != "api_key" || principal.HasScope(scope)
}

func (s *Server) capture(ctx context.Context, incident model.Incident) {
	tracker, _ := ctx.Value(incidentCaptureTrackerKey{}).(*incidentCaptureTracker)
	if tracker != nil && !tracker.captured.CompareAndSwap(false, true) {
		return
	}
	ctx = context.WithoutCancel(ctx)
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if user, ok := UserFromContext(ctx); ok {
		incident.UserID = &user.ID
	}
	captureIncident := s.captureIncident
	if captureIncident == nil && s.store != nil {
		captureIncident = s.store.CaptureIncident
	}
	if captureIncident == nil {
		if tracker != nil {
			tracker.captured.Store(false)
		}
		s.logger.Error("capture incident failed", "error", "incident store is unavailable", "request_id", incident.RequestID)
		return
	}
	if err := captureIncident(ctx, incident); err != nil {
		if tracker != nil {
			tracker.captured.Store(false)
		}
		s.logger.Error("capture incident failed", "error", err, "request_id", incident.RequestID)
	}
}

func (s *Server) internalError(writer http.ResponseWriter, request *http.Request, code string, cause error) {
	s.logger.Error("request failed", "request_id", RequestID(request.Context()), "error", cause)
	details, _ := json.Marshal(map[string]any{"code": code})
	s.capture(request.Context(), model.Incident{RequestID: RequestID(request.Context()), Kind: "request", Severity: "error", Message: cause.Error(), Details: details})
	writeError(writer, request, http.StatusInternalServerError, code, "The server could not complete the request", nil)
}

func randomRequestID() string {
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("req-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer)
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

func containsAnyFold(values, targets []string) bool {
	for _, value := range values {
		if containsFold(targets, value) {
			return true
		}
	}
	return false
}
