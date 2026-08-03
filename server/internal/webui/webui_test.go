package webui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func workspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"index.html":              "<!doctype html><title>Ptium</title>",
		"assets/index-abc123.js":  "console.log('ptium')",
		"assets/index-abc123.css": ":root{}",
		"favicon.svg":             "<svg/>",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestHandlerRequiresBuiltAssets(t *testing.T) {
	if _, err := Handler(""); err == nil {
		t.Fatal("an empty directory must be rejected")
	}
	if _, err := Handler(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("a missing directory must be rejected")
	}
	empty := t.TempDir()
	if _, err := Handler(empty); err == nil {
		t.Fatal("a directory without index.html must be rejected")
	}
}

func TestHandlerServesAssetsAndFallsBackToIndex(t *testing.T) {
	handler, err := Handler(workspace(t))
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}
	cases := []struct {
		path        string
		wantBody    string
		wantCache   string
		wantContent string
	}{
		{path: "/assets/index-abc123.js", wantBody: "console.log('ptium')", wantCache: "public, max-age=31536000, immutable"},
		{path: "/favicon.svg", wantBody: "<svg/>", wantCache: "no-cache"},
		{path: "/", wantBody: "<!doctype html><title>Ptium</title>", wantCache: "no-store", wantContent: "text/html; charset=utf-8"},
		// Client-side routes must survive a hard reload.
		{path: "/presentations/8f14e45f-ceea-467a-9f26-2f39cbcbe0a3/editor", wantBody: "<!doctype html><title>Ptium</title>", wantCache: "no-store"},
		{path: "/admin/settings", wantBody: "<!doctype html><title>Ptium</title>", wantCache: "no-store"},
	}
	for _, test := range cases {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status = %d", test.path, recorder.Code)
		}
		if recorder.Body.String() != test.wantBody {
			t.Fatalf("%s body = %q, want %q", test.path, recorder.Body.String(), test.wantBody)
		}
		if got := recorder.Header().Get("Cache-Control"); got != test.wantCache {
			t.Fatalf("%s cache-control = %q, want %q", test.path, got, test.wantCache)
		}
		if test.wantContent != "" && recorder.Header().Get("Content-Type") != test.wantContent {
			t.Fatalf("%s content-type = %q", test.path, recorder.Header().Get("Content-Type"))
		}
		for header, want := range map[string]string{
			"X-Content-Type-Options": "nosniff",
			"X-Frame-Options":        "DENY",
			"Referrer-Policy":        "same-origin",
		} {
			if got := recorder.Header().Get(header); got != want {
				t.Fatalf("%s %s = %q, want %q", test.path, header, got, want)
			}
		}
		if recorder.Header().Get("Content-Security-Policy") == "" {
			t.Fatalf("%s is missing a content security policy", test.path)
		}
	}
}

func TestHandlerRejectsWritesAndPathEscapes(t *testing.T) {
	handler, err := Handler(workspace(t))
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want 405", recorder.Code)
	}
	// A traversal attempt resolves inside the root and falls back to index.html
	// rather than reaching the filesystem above it.
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/../../etc/passwd", nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "<!doctype html><title>Ptium</title>" {
		t.Fatalf("traversal returned %d %q", recorder.Code, recorder.Body.String())
	}
}
