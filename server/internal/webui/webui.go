// Package webui serves the compiled single-page workspace from the Go process,
// so a deployment is one container listening on one port with no reverse proxy
// in front of it.
package webui

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

// contentSecurityPolicy matches what the workspace actually needs: its own
// bundle, inline styles from the CSS-in-JS runtime, images from anywhere (logo
// URLs are administrator-configured) and API calls to its own origin.
const contentSecurityPolicy = "default-src 'self'; base-uri 'self'; object-src 'none'; " +
	"frame-ancestors 'none'; form-action 'self'; script-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; img-src 'self' data: blob: http: https:; " +
	"font-src 'self' data:; connect-src 'self' http: https:"

// Handler serves the workspace from a directory of built assets. Requests for
// hashed asset files are cached immutably; every other path falls back to
// index.html so client-side routing works on a hard reload.
func Handler(directory string) (http.Handler, error) {
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
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		writer.Header().Set("Cache-Control", "no-store")
		writer.WriteHeader(http.StatusOK)
		if request.Method == http.MethodHead {
			return
		}
		_, _ = writer.Write(index)
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

func exists(root fs.FS, name string) bool {
	info, err := fs.Stat(root, name)
	return err == nil && !info.IsDir()
}
