package webui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/analytics"
)

func trackedWorkspace(t *testing.T) string {
	t.Helper()
	root := workspace(t)
	page := "<!DOCTYPE html><html><HEAD><title>Ptium</title></HEAD><body><div id=\"root\"></div></body></html>"
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func serve(t *testing.T, config analytics.Config, target string) *httptest.ResponseRecorder {
	t.Helper()
	handler, err := Handler(trackedWorkspace(t), func(*http.Request) analytics.Config { return config })
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	return recorder
}

var nonceInTag = regexp.MustCompile(`<script[^>]*\snonce="([^"]+)"`)

// directive is one directive's sources out of a policy.
func directive(policy, name string) string {
	for _, part := range strings.Split(policy, ";") {
		if fields := strings.Fields(part); len(fields) > 0 && fields[0] == name {
			return strings.Join(fields[1:], " ")
		}
	}
	return ""
}

func TestAFreshInstallationCarriesNoSnippetAndTheSamePolicy(t *testing.T) {
	for _, config := range []analytics.Config{
		{},
		{Enabled: false, Provider: analytics.ProviderMomento, MomentoURL: "https://m.example", MomentoSiteID: "s"},
		{Enabled: true, Provider: analytics.ProviderNone},
		{Enabled: true, Provider: analytics.ProviderMomento},
	} {
		for _, target := range []string{"/", "/dashboard", "/admin/settings"} {
			recorder := serve(t, config, target)
			if strings.Contains(recorder.Body.String(), "<script") {
				t.Fatalf("%+v %s: a snippet was served: %s", config, target, recorder.Body.String())
			}
			if got := recorder.Header().Get("Content-Security-Policy"); got != contentSecurityPolicy {
				t.Fatalf("%+v %s: policy changed while tracking is off: %s", config, target, got)
			}
		}
	}
}

func TestTheSnippetGoesWhereAskedWithTheRequestsNonceInThePolicy(t *testing.T) {
	config := analytics.Config{Enabled: true, Provider: analytics.ProviderCustom,
		CustomSnippet: `<script src="https://cdn.tracker.example/t.js"></script><script>t('https://collect.tracker.example')</script>`}
	seen := map[string]bool{}
	for _, placement := range []string{analytics.PlacementHead, analytics.PlacementBody} {
		config.Placement = placement
		recorder := serve(t, config, "/dashboard")
		body := recorder.Body.String()
		policy := recorder.Header().Get("Content-Security-Policy")
		nonces := nonceInTag.FindAllStringSubmatch(body, -1)
		if len(nonces) != 2 || nonces[0][1] != nonces[1][1] {
			t.Fatalf("%s: nonces on the tags = %v", placement, nonces)
		}
		nonce := nonces[0][1]
		if len(nonce) < 22 || seen[nonce] {
			t.Fatalf("%s: nonce %q is short or repeated", placement, nonce)
		}
		seen[nonce] = true
		if !strings.Contains(policy, "script-src 'self' 'nonce-"+nonce+"' https://cdn.tracker.example https://collect.tracker.example;") {
			t.Fatalf("%s: policy lacks the nonce or the snippet's origins: %s", placement, policy)
		}
		if strings.Contains(directive(policy, "script-src"), "unsafe-inline") {
			t.Fatalf("%s: script policy was loosened: %s", placement, policy)
		}
		if !strings.HasSuffix(policy, "report-uri "+ReportPath) {
			t.Fatalf("%s: no report-uri while tracking is on: %s", placement, policy)
		}
		closing := "</HEAD>"
		if placement == analytics.PlacementBody {
			closing = "</body>"
		}
		at := strings.Index(body, "<script")
		if at < 0 || strings.Index(body, closing) < at || (placement == analytics.PlacementHead && strings.Index(body, "<body>") < at) {
			t.Fatalf("%s: the snippet is not just before %s: %s", placement, closing, body)
		}
	}
}

func TestTheAdminScreensAreTrackedOnlyWhenAskedFor(t *testing.T) {
	config := analytics.Config{Enabled: true, Provider: analytics.ProviderGA4, MeasurementID: "G-1"}
	if recorder := serve(t, config, "/admin/settings"); strings.Contains(recorder.Body.String(), "<script") || recorder.Header().Get("Content-Security-Policy") != contentSecurityPolicy {
		t.Fatalf("admin page tracked without include_admin: %s", recorder.Body.String())
	}
	if recorder := serve(t, config, "/dashboard"); !strings.Contains(recorder.Body.String(), "googletagmanager") {
		t.Fatalf("workspace page not tracked: %s", recorder.Body.String())
	}
	config.IncludeAdmin = true
	if recorder := serve(t, config, "/admin/settings"); !strings.Contains(recorder.Body.String(), "googletagmanager") {
		t.Fatalf("admin page not tracked with include_admin: %s", recorder.Body.String())
	}
}

func TestAssetsKeepTheStrictPolicyAndNoSnippet(t *testing.T) {
	config := analytics.Config{Enabled: true, Provider: analytics.ProviderGA4, MeasurementID: "G-1"}
	recorder := serve(t, config, "/assets/index-abc123.js")
	if recorder.Body.String() != "console.log('ptium')" || recorder.Header().Get("Content-Security-Policy") != contentSecurityPolicy {
		t.Fatalf("asset = %q policy = %q", recorder.Body.String(), recorder.Header().Get("Content-Security-Policy"))
	}
}

func TestAProxiedMomentoAddsNoOriginToThePolicy(t *testing.T) {
	config := analytics.Config{Enabled: true, Provider: analytics.ProviderMomento, MomentoURL: "https://momento.corp.example", MomentoSiteID: "ptium", MomentoProxy: true}
	recorder := serve(t, config, "/")
	policy := recorder.Header().Get("Content-Security-Policy")
	if strings.Contains(policy, "momento.corp.example") {
		t.Fatalf("the collector reached the policy: %s", policy)
	}
	if !strings.Contains(recorder.Body.String(), `src="/momento/tracker.js"`) {
		t.Fatalf("the tracker is not fetched through this origin: %s", recorder.Body.String())
	}
	config.MomentoProxy = false
	policy = serve(t, config, "/").Header().Get("Content-Security-Policy")
	if !strings.Contains(policy, "https://momento.corp.example;") {
		t.Fatalf("a direct collector is not in script-src: %s", policy)
	}
}

func TestAPageWithoutAClosingTagStillGetsTheSnippet(t *testing.T) {
	got := string(injectSnippet([]byte("<html>bare"), "<script>x</script>", analytics.PlacementHead))
	if got != "<html>bare\n<script>x</script>\n" {
		t.Fatalf("got %q", got)
	}
	if got := string(injectSnippet([]byte("<p>K</p></HEAD>"), "<s/>", analytics.PlacementHead)); got != "<p>K</p><s/>\n</HEAD>" {
		t.Fatalf("a Kelvin sign moved the marker: %q", got)
	}
}
