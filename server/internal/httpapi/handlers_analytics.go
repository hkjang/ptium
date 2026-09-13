package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
	"strings"

	"github.com/hkjang/ptium/server/internal/analytics"
	"github.com/hkjang/ptium/server/internal/settings"
)

// maxReportBytes keeps an unauthenticated endpoint from being used to push
// large bodies at the server. A browser's report is a few hundred bytes.
const maxReportBytes = 8 * 1024

// apiPolicy is what every non-page answer carries. Nothing under /api, /mcp or
// the health checks is a document, so nothing in one may load anything; a
// handler that does send a document — an SVG preview — sets its own.
const apiPolicy = "default-src 'none'; frame-ancestors 'none'"

// tracking is the administrator's tracking configuration, read when a page is
// served rather than at startup, so a switch flipped on the settings screen
// moves without a restart. A settings outage reads as "off".
func (s *Server) tracking(request *http.Request) analytics.Config {
	if s.readTracking == nil {
		return analytics.Config{}
	}
	return s.readTracking(request.Context())
}

// cspReport is the shape a browser posts to report-uri.
type cspReport struct {
	Report struct {
		BlockedURI         string `json:"blocked-uri"`
		ViolatedDirective  string `json:"violated-directive"`
		EffectiveDirective string `json:"effective-directive"`
		DocumentURI        string `json:"document-uri"`
	} `json:"csp-report"`
}

// receiveCSPReport records what a browser refused to load. It is reachable
// without credentials because the browser sends it without any, and it keeps
// nothing but a bounded list of origins in memory. Reports are always answered
// 204, so a misbehaving page never sees an error from us.
func (s *Server) receiveCSPReport(writer http.ResponseWriter, request *http.Request) {
	defer writer.WriteHeader(http.StatusNoContent)
	if s.violations == nil {
		return
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, maxReportBytes))
	if err != nil || len(body) == 0 {
		return
	}
	var report cspReport
	if json.Unmarshal(body, &report) != nil {
		return
	}
	directive := report.Report.EffectiveDirective
	if directive == "" {
		directive = report.Report.ViolatedDirective
	}
	s.violations.Record(report.Report.BlockedURI, directive, report.Report.DocumentURI)
}

// adminListViolations shows the administrator which addresses the page policy
// is refusing, so a tracking snippet can be fixed without reading a browser
// console.
func (s *Server) adminListViolations(writer http.ResponseWriter, request *http.Request) {
	items := []analytics.Violation{}
	if s.violations != nil {
		items = s.violations.List(s.tracking(request))
	}
	writeData(writer, request, http.StatusOK, items)
}

// adminForgetViolations drops the recorded reports, which is how an
// administrator checks whether a change actually fixed the snippet.
func (s *Server) adminForgetViolations(writer http.ResponseWriter, request *http.Request) {
	if s.violations != nil {
		s.violations.Forget()
	}
	writer.WriteHeader(http.StatusNoContent)
}

// adminAllowOrigin adds one blocked origin to analytics.allowed_hosts. It is
// the one-click fix for the reports listed above, and goes through the same
// validation and trail as typing the origin into the settings screen.
func (s *Server) adminAllowOrigin(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Origin string `json:"origin"`
	}
	if !decodeJSON(writer, request, &input) {
		return
	}
	origin := strings.TrimSpace(input.Origin)
	if origin == "" || !(strings.HasPrefix(strings.ToLower(origin), "http://") || strings.HasPrefix(strings.ToLower(origin), "https://")) {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", "origin must be an HTTP(S) origin", nil)
		return
	}
	current := s.tracking(request).AllowedHosts
	value, err := json.Marshal(analytics.AddAllowedHost(current, origin))
	if err != nil {
		s.internalError(writer, request, "admin_setting_update_failed", err)
		return
	}
	user, _ := UserFromContext(request.Context())
	was := s.settingsNow(request.Context())
	update := settingUpdate{Key: "analytics.allowed_hosts", Value: value}
	setting, ok := s.putSetting(writer, request, user.ID, update)
	if !ok {
		return
	}
	s.auditSettingChange(request.Context(), user.ID, settings.Update{Key: update.Key, Value: update.Value}, was[update.Key])
	s.store.Audit(request.Context(), &user.ID, "settings.update", "setting", update.Key, map[string]any{"origin": origin})
	writeData(writer, request, http.StatusOK, setting)
}

// momentoProxy forwards /momento/* to the Momento collector while the
// administrator has chosen to reach it through this origin. The browser then
// loads the tracker from and reports to the workspace's own address, so the
// collector never has to appear in the page policy — which is what lets a
// deployment whose policy cannot be changed still count its visitors.
//
// It answers 404 whenever tracking is off, another provider is chosen, or the
// proxy is turned off, so a fresh installation has nothing listening here.
func (s *Server) momentoProxy(writer http.ResponseWriter, request *http.Request) {
	target := s.tracking(request).ProxyTarget()
	if target == nil {
		http.NotFound(writer, request)
		return
	}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(proxied *httputil.ProxyRequest) {
			proxied.SetURL(target)
			proxied.Out.URL.Path = strings.TrimPrefix(request.URL.Path, analytics.ProxyPath)
			proxied.Out.URL.RawPath = ""
			proxied.SetXForwarded()
			// What the browser sends to its own origin is not the collector's
			// business: the session cookie above all, and any bearer token.
			proxied.Out.Header.Del("Cookie")
			proxied.Out.Header.Del("Authorization")
			proxied.Out.Header.Del("X-Ptium-Dev-Secret")
			proxied.Out.Header.Del("X-API-Key")
		},
		ErrorHandler: func(writer http.ResponseWriter, request *http.Request, err error) {
			s.logger.Warn("momento collector unreachable", "request_id", RequestID(request.Context()), "error", err)
			writer.WriteHeader(http.StatusBadGateway)
		},
	}
	request.Body = http.MaxBytesReader(writer, request.Body, maxProxiedBytes)
	proxy.ServeHTTP(writer, request)
}

// maxProxiedBytes bounds what one event post may carry to the collector.
const maxProxiedBytes = 256 * 1024
