package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/analytics"
	"github.com/hkjang/ptium/server/internal/webui"
)

func trackingServer(config analytics.Config) *Server {
	return &Server{
		logger:       slog.New(slog.DiscardHandler),
		violations:   analytics.NewRecorder(),
		readTracking: func(context.Context) analytics.Config { return config },
	}
}

func TestABrowsersReportIsKeptAsAnOriginAndADirective(t *testing.T) {
	server := trackingServer(analytics.Config{})
	body := `{"csp-report":{"blocked-uri":"https://momento.corp.example/collect/v1/events","effective-directive":"connect-src","document-uri":"https://slides.corp.example/dashboard"}}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, webui.ReportPath, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/csp-report")
	server.receiveCSPReport(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d", recorder.Code)
	}
	items := server.violations.List(analytics.Config{})
	if len(items) != 1 || items[0].Origin != "https://momento.corp.example" || items[0].Directive != "connect-src" || items[0].Page != "https://slides.corp.example/dashboard" {
		t.Fatalf("items = %+v", items)
	}
	// The same origin again is the same line, not another.
	server.receiveCSPReport(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, webui.ReportPath, strings.NewReader(body)))
	if items = server.violations.List(analytics.Config{}); len(items) != 1 || items[0].Count != 2 {
		t.Fatalf("repeated report: %+v", items)
	}
	// Nonsense is swallowed: a page must never see an error from this.
	for _, junk := range []string{"not json", "", `{"csp-report":{"blocked-uri":"inline"}}`, strings.Repeat("x", 20000)} {
		broken := httptest.NewRecorder()
		server.receiveCSPReport(broken, httptest.NewRequest(http.MethodPost, webui.ReportPath, strings.NewReader(junk)))
		if broken.Code != http.StatusNoContent {
			t.Fatalf("%q answered %d", junk[:min(len(junk), 20)], broken.Code)
		}
	}
	if items = server.violations.List(analytics.Config{}); len(items) != 1 {
		t.Fatalf("junk was recorded: %+v", items)
	}
}

func TestTheReportPathAnswersWithoutCredentialsAndDataPathsCarryANarrowPolicy(t *testing.T) {
	// The route is registered as a literal so the API description can be
	// checked against it; the policy names the same path through webui.
	if webui.ReportPath != "/api/v1/analytics/csp-report" {
		t.Fatalf("the policy reports to %s, which nothing answers", webui.ReportPath)
	}
	server := trackingServer(analytics.Config{})
	// The routing table wants the whole server; what matters here is that the
	// report path and the proxy are registered on the open mux, before the
	// authentication that guards everything else under /api.
	root := http.NewServeMux()
	root.HandleFunc("POST "+webui.ReportPath, server.receiveCSPReport)
	root.HandleFunc(analytics.ProxyPath+"/", server.momentoProxy)
	handler := server.requestMiddleware(root)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, webui.ReportPath, strings.NewReader(`{"csp-report":{"blocked-uri":"https://t.example/x.js","violated-directive":"script-src-elem"}}`)))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("report answered %d", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Security-Policy"); got != apiPolicy {
		t.Fatalf("a data path carries %q", got)
	}
	for path, narrow := range map[string]bool{"/api/v1/me": true, "/mcp": true, "/healthz": true, "/readyz": true, "/auth/me": true, "/": false, "/dashboard": false, "/momento/tracker.js": false, "/apiary": false} {
		if isNotAPage(path) != narrow {
			t.Errorf("isNotAPage(%q) = %v", path, !narrow)
		}
	}

	// Nothing listens at the proxy on a fresh installation.
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/momento/tracker.js", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("the proxy answered %d while tracking is off", recorder.Code)
	}
}

func TestTheProxyReachesTheCollectorWithoutTheBrowsersCredentials(t *testing.T) {
	var seenPath, seenCookie, seenAuthorization, seenForwarded string
	collector := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seenPath = request.URL.Path + "?" + request.URL.RawQuery
		seenCookie = request.Header.Get("Cookie")
		seenAuthorization = request.Header.Get("Authorization")
		seenForwarded = request.Header.Get("X-Forwarded-Host")
		writer.Header().Set("Content-Type", "application/javascript")
		_, _ = writer.Write([]byte("tracker()"))
	}))
	defer collector.Close()

	config := analytics.Config{Enabled: true, Provider: analytics.ProviderMomento, MomentoURL: collector.URL + "/", MomentoSiteID: "ptium", MomentoProxy: true}
	server := trackingServer(config)
	request := httptest.NewRequest(http.MethodGet, "/momento/tracker.js?v=1", nil)
	request.Host = "slides.corp.example"
	request.Header.Set("Cookie", "ptium_session=secret")
	request.Header.Set("Authorization", "Bearer secret")
	recorder := httptest.NewRecorder()
	server.momentoProxy(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "tracker()" {
		t.Fatalf("proxied answer = %d %q", recorder.Code, recorder.Body.String())
	}
	if seenPath != "/tracker.js?v=1" {
		t.Fatalf("collector saw path %q", seenPath)
	}
	if seenCookie != "" || seenAuthorization != "" {
		t.Fatalf("the collector was handed the browser's credentials: cookie=%q authorization=%q", seenCookie, seenAuthorization)
	}
	if seenForwarded != "slides.corp.example" {
		t.Fatalf("X-Forwarded-Host = %q", seenForwarded)
	}

	// Told to go direct, the proxy no longer forwards.
	config.MomentoProxy = false
	recorder = httptest.NewRecorder()
	trackingServer(config).momentoProxy(recorder, httptest.NewRequest(http.MethodGet, "/momento/tracker.js", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("direct mode still proxied: %d", recorder.Code)
	}
	// A collector that is down is a 502, not a panic and not a hang.
	collector.Close()
	config.MomentoProxy = true
	recorder = httptest.NewRecorder()
	trackingServer(config).momentoProxy(recorder, httptest.NewRequest(http.MethodPost, "/momento/collect/v1/events", strings.NewReader("{}")))
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("a dead collector answered %d", recorder.Code)
	}
}

func TestAnOriginToAllowMustBeAnHTTPOrigin(t *testing.T) {
	server := trackingServer(analytics.Config{})
	for _, body := range []string{`{"origin":""}`, `{"origin":"ftp://x"}`, `{"origin":"momento.corp.example"}`, `{}`} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/analytics/allow", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		server.adminAllowOrigin(recorder, request)
		if recorder.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s answered %d", body, recorder.Code)
		}
	}
}

func TestTheTrackingSettingsAreRefusedOutsideWhatThePageHonours(t *testing.T) {
	cases := []struct {
		key   string
		value string
		ok    bool
	}{
		{"analytics.custom_snippet", `"<script src='https://t.example/t.js'></script>"`, true},
		{"analytics.custom_snippet", `"` + strings.Repeat("x", analytics.MaxSnippetBytes+1) + `"`, false},
		{"analytics.custom_snippet", `"` + strings.Repeat("x", analytics.MaxSnippetBytes) + `"`, true},
		{"analytics.custom_snippet", `12`, false},
		{"analytics.momento_url", `""`, true},
		{"analytics.momento_url", `"https://momento.internal"`, true},
		{"analytics.momento_url", `"http://momento.internal:8080/collect"`, true},
		{"analytics.momento_url", `"momento.internal"`, false},
		{"analytics.momento_url", `"https://user:pw@momento.internal"`, false},
		{"analytics.matomo_url", `"ftp://matomo"`, false},
		{"analytics.allowed_hosts", `""`, true},
		{"analytics.allowed_hosts", `"https://a.example, https://b.example:8443"`, true},
		{"analytics.allowed_hosts", `"https://a.example/path"`, false},
		{"analytics.allowed_hosts", `"a.example"`, false},
		{"analytics.measurement_id", `"G-ABC123"`, true},
		{"analytics.measurement_id", `"` + strings.Repeat("G", 201) + `"`, false},
		{"analytics.provider", `"momento"`, true},
		{"analytics.provider", `"piwik"`, false},
		{"analytics.placement", `"body"`, true},
		{"analytics.placement", `"footer"`, false},
		{"analytics.enabled", `true`, true},
		{"analytics.enabled", `"true"`, false},
	}
	for _, test := range cases {
		err := validateSettingValue(test.key, []byte(test.value))
		if (err == nil) != test.ok {
			t.Errorf("%s = %.40s: err = %v, want ok=%v", test.key, test.value, err, test.ok)
		}
	}
}
