// Package webui serves the compiled single-page workspace from the Go process,
// so a deployment is one container listening on one port with no reverse proxy
// in front of it.
package webui

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/hkjang/ptium/server/internal/analytics"
)

// contentSecurityPolicy matches what the workspace actually needs: its own
// bundle, inline styles from the CSS-in-JS runtime, images from anywhere (logo
// URLs are administrator-configured) and API calls to its own origin.
//
// It is what every page carries while tracking is off, and what every asset
// carries regardless. A page with tracking on gets policyFor instead: the same
// policy with a nonce and the tracker's origins added, never 'unsafe-inline'.
const contentSecurityPolicy = "default-src 'self'; base-uri 'self'; object-src 'none'; " +
	"frame-ancestors 'none'; form-action 'self'; script-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; img-src 'self' data: blob: http: https:; " +
	"font-src 'self' data:; connect-src 'self' http: https:"

// ReportPath is where a browser posts what the page policy refused. It is put
// in the policy only while tracking is on, so a strict deployment reports
// nothing.
const ReportPath = "/api/v1/analytics/csp-report"

// Tracking answers, per request, how the administrator has configured visitor
// tracking. Nil means none, which is what a fresh installation has.
type Tracking func(request *http.Request) analytics.Config

// Handler serves the workspace from a directory of built assets. Requests for
// hashed asset files are cached immutably; every other path falls back to
// index.html so client-side routing works on a hard reload.
func Handler(directory string, tracking Tracking) (http.Handler, error) {
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return nil, errors.New("web asset directory is empty")
	}
	info, err := os.Stat(directory)
	if err != nil {
		return nil, fmt.Errorf("read web asset directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", directory)
	}
	root := os.DirFS(directory)
	index, err := fs.ReadFile(root, "index.html")
	if err != nil {
		return nil, fmt.Errorf("read %s/index.html: %w", directory, err)
	}
	server := http.FileServerFS(root)

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			writer.Header().Set("Allow", "GET, HEAD")
			http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+request.URL.Path), "/")
		securityHeaders(writer)
		if name != "" && name != "index.html" && exists(root, name) {
			if strings.HasPrefix(name, "assets/") {
				// Vite fingerprints these filenames, so they never change content.
				writer.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				writer.Header().Set("Cache-Control", "no-cache")
			}
			server.ServeHTTP(writer, request)
			return
		}
		page := index
		if tracking != nil {
			config := tracking(request)
			if config.Active(request.URL.Path) {
				nonce := newNonce()
				page = injectSnippet(index, config.Snippet(nonce), config.Placement)
				writer.Header().Set("Content-Security-Policy", policyFor(config, nonce))
			}
		}
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		writer.Header().Set("Cache-Control", "no-store")
		writer.WriteHeader(http.StatusOK)
		if request.Method == http.MethodHead {
			return
		}
		_, _ = writer.Write(page)
	}), nil
}

func securityHeaders(writer http.ResponseWriter) {
	header := writer.Header()
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("X-Frame-Options", "DENY")
	header.Set("Referrer-Policy", "same-origin")
	header.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
	header.Set("Content-Security-Policy", contentSecurityPolicy)
}

// policyFor is the strict page policy with only what the configured tracker
// needs added: the request's nonce, so the snippet's inline code may run, and
// the origins the tracker loads from and reports to. While tracking is on the
// browser is also asked to say what it refused, which is what turns a console
// error into a line on the settings screen.
func policyFor(config analytics.Config, nonce string) string {
	scripts := []string{"'self'", "'nonce-" + nonce + "'"}
	connects := []string{"'self'", "http:", "https:"}
	images := []string{"'self'", "data:", "blob:", "http:", "https:"}
	extraScripts, extraConnects, extraImages := config.PolicySources()
	scripts = append(scripts, extraScripts...)
	connects = append(connects, extraConnects...)
	images = append(images, extraImages...)
	return "default-src 'self'; base-uri 'self'; object-src 'none'; " +
		"frame-ancestors 'none'; form-action 'self'; script-src " + strings.Join(scripts, " ") + "; " +
		"style-src 'self' 'unsafe-inline'; img-src " + strings.Join(images, " ") + "; " +
		"font-src 'self' data:; connect-src " + strings.Join(connects, " ") + "; " +
		"report-uri " + ReportPath
}

// newNonce is one request's nonce: 128 bits, which is what the policy
// specification asks for, in the base64 the header expects.
func newNonce() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		// The system's randomness failing is not something a page can recover
		// from; a nonce nothing matches simply keeps the snippet from running.
		return "unavailable"
	}
	return base64.StdEncoding.EncodeToString(raw)
}

// injectSnippet places the markup just before the closing tag it belongs to,
// falling back to the end of the document when the tag is missing.
func injectSnippet(page []byte, snippet, placement string) []byte {
	if strings.TrimSpace(snippet) == "" {
		return page
	}
	marker := "</head>"
	if placement == analytics.PlacementBody {
		marker = "</body>"
	}
	text := string(page)
	index := lastIndexFold(text, marker)
	if index < 0 {
		return []byte(text + "\n" + snippet + "\n")
	}
	return []byte(text[:index] + snippet + "\n" + text[index:])
}

// lastIndexFold finds the last marker ignoring ASCII case, without folding
// the page: strings.ToLower changes byte lengths for some letters, and an
// index taken from the folded copy would land elsewhere in the original.
func lastIndexFold(text, marker string) int {
	for i := len(text) - len(marker); i >= 0; i-- {
		if strings.EqualFold(text[i:i+len(marker)], marker) {
			return i
		}
	}
	return -1
}

func exists(root fs.FS, name string) bool {
	info, err := fs.Stat(root, name)
	return err == nil && !info.IsDir()
}
