package handoff

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestThePeerListIsReadAsNameAndOriginAndNothingLooser(t *testing.T) {
	config, err := ParsePeers([]string{" Weekly = HTTPS://Weekly.Intra/ ", "", "umm=http://umm.intra:8080", "other=https://x.intra"})
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Peers) != 3 {
		t.Fatalf("peers = %+v", config.Peers)
	}
	if got := config.Peers[0]; got.Name != "weekly" || got.Origin != "https://weekly.intra" || strings.Join(got.Receives, ",") != "markdown,docx,pptx" {
		t.Fatalf("weekly = %+v", got)
	}
	if got := config.Peers[1]; got.Origin != "http://umm.intra:8080" || len(got.Receives) != 0 {
		t.Fatalf("umm = %+v", got)
	}
	// A name the table does not know is a peer this service only receives from.
	if got := config.Peers[2]; len(got.Receives) != 0 {
		t.Fatalf("other = %+v", got)
	}
	for _, bad := range [][]string{
		{"https://weekly.intra"},
		{"weekly="},
		{"weekly=weekly.intra"},
		{"weekly=ftp://weekly.intra"},
		{"weekly=https://weekly.intra/handoff"},
		{"weekly=https://weekly.intra?x=1"},
		{"weekly=https://user@weekly.intra"},
		{"weekly=https://weekly.intra", "muni=https://weekly.intra"},
		{"we ekly=https://weekly.intra"},
	} {
		if _, err := ParsePeers(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestOnlyAnExactOriginOnTheListIsAllowed(t *testing.T) {
	config, _ := ParsePeers([]string{"umm=https://umm.intra"})
	for source, want := range map[string]bool{
		"https://umm.intra":             true,
		"HTTPS://UMM.INTRA/":            true,
		"https://umm.intra/":            true,
		"https://umm.intra/../other":    false,
		"https://umm.intra?x=1":         false,
		"https://umm.intra#f":           false,
		"https://a@umm.intra":           false,
		"http://umm.intra":              false,
		"https://umm.intra:8443":        false,
		"https://umm.intra.evil.test":   false,
		"https://evil.test/umm.intra":   false,
		"":                              false,
		"umm.intra":                     false,
		"javascript:alert(1)":           false,
		"https://umm.intra\\@evil.test": false,
	} {
		if _, got := config.Allowed(source); got != want {
			t.Errorf("Allowed(%q) = %v", source, got)
		}
	}
}

func TestADestinationIsShownOnlyForAFormatItReceives(t *testing.T) {
	config, _ := ParsePeers([]string{"umm=https://umm.intra", "muni=https://muni.intra", "kanpic=https://kanpic.intra", "weekly=https://weekly.intra", "other=https://other.intra"})
	targets := config.Targets()
	if len(targets) != 1 || targets[0].Name != "weekly" || targets[0].Origin != "https://weekly.intra" || strings.Join(targets[0].Formats, ",") != "pptx" {
		t.Fatalf("targets = %+v", targets)
	}
	if got := len(Config{}.Targets()); got != 0 {
		t.Fatalf("an empty list has %d targets", got)
	}
	if got := ReceiveURL("https://weekly.intra", "https://slides.intra", "abc_DEF-123456789"); got != "https://weekly.intra/handoff?claim=abc_DEF-123456789&source=https%3A%2F%2Fslides.intra" {
		t.Fatalf("url = %s", got)
	}
}

func TestAClaimIsRandomAndLooksLikeOne(t *testing.T) {
	first, err := NewClaim()
	if err != nil {
		t.Fatal(err)
	}
	second, _ := NewClaim()
	if first == second || len(first) < 40 || !ValidClaim(first) {
		t.Fatalf("claims %q %q", first, second)
	}
	for claim, want := range map[string]bool{
		"abcdefghijklmnop":       true,
		"abc":                    false,
		"abcdefghijklmno/":       false,
		"abcdefghijklmn..":       false,
		strings.Repeat("a", 257): false,
		"한글한글한글한글한글한글한글한글한글": false,
	} {
		if ValidClaim(claim) != want {
			t.Errorf("ValidClaim(%q) = %v", claim, !want)
		}
	}
}

func peerServing(t *testing.T, handler http.HandlerFunc) (*httptest.Server, Config, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	config, err := ParsePeers([]string{"umm=" + server.URL})
	if err != nil {
		t.Fatal(err)
	}
	return server, config, &hits
}

const claim = "abcdefghijklmnopqrstuvwxyz012345"

func TestASourceOffTheListCostsNoRequest(t *testing.T) {
	server, _, hits := peerServing(t, func(w http.ResponseWriter, r *http.Request) {})
	empty := Config{}
	if _, err := Fetch(context.Background(), NewClient(nil), empty, server.URL, claim); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("err = %v", err)
	}
	other, _ := ParsePeers([]string{"muni=https://muni.intra"})
	if _, err := Fetch(context.Background(), NewClient(nil), other, server.URL, claim); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("err = %v", err)
	}
	// On the list, but the claim is not a claim: still no request.
	listed, _ := ParsePeers([]string{"umm=" + server.URL})
	if _, err := Fetch(context.Background(), NewClient(nil), listed, server.URL, "../admin"); !errors.Is(err, ErrBadClaim) {
		t.Fatalf("err = %v", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("the peer was asked %d times", hits.Load())
	}
}

func TestAFetchTakesTheDocumentAndItsName(t *testing.T) {
	server, config, hits := peerServing(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != ClaimsPath+"/"+claim {
			t.Errorf("path = %s", r.URL.Path)
		}
		if !strings.Contains(r.Header.Get("Accept"), "text/markdown") {
			t.Errorf("accept = %s", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", Disposition("2026년 3분기 개편안.md"))
		_, _ = w.Write([]byte("# 개편안\n\n본문"))
	})
	document, err := Fetch(context.Background(), NewClient(nil), config, server.URL, claim)
	if err != nil {
		t.Fatal(err)
	}
	if document.Format != FormatMarkdown || document.Filename != "2026년 3분기 개편안.md" || string(document.Body) != "# 개편안\n\n본문" {
		t.Fatalf("document = %+v", document)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits = %d", hits.Load())
	}
}

func TestANamelessOrMisnamedDocumentGetsItsFormatsSuffix(t *testing.T) {
	for _, tc := range []struct{ disposition, contentType, want string }{
		{"", "text/csv", "handoff.csv"},
		{`attachment; filename="report"`, "text/plain", "report.txt"},
		{`attachment; filename="report.TXT"`, "text/plain", "report.TXT"},
		{`attachment; filename="../../etc/passwd"`, "text/markdown", "passwd.md"},
		{`attachment; filename="a\\b.docx"`, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "b.docx"},
	} {
		server, config, _ := peerServing(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", tc.contentType)
			if tc.disposition != "" {
				w.Header().Set("Content-Disposition", tc.disposition)
			}
			_, _ = w.Write([]byte("x"))
		})
		document, err := Fetch(context.Background(), NewClient(nil), config, server.URL, claim)
		if err != nil {
			t.Fatalf("%+v: %v", tc, err)
		}
		if document.Filename != tc.want {
			t.Errorf("%+v: filename = %q", tc, document.Filename)
		}
	}
}

func TestARedirectIsNotFollowed(t *testing.T) {
	var elsewhere atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { elsewhere.Add(1) }))
	defer other.Close()
	server, config, _ := peerServing(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/secret", http.StatusFound)
	})
	if _, err := Fetch(context.Background(), NewClient(nil), config, server.URL, claim); !errors.Is(err, ErrRedirect) {
		t.Fatalf("err = %v", err)
	}
	if elsewhere.Load() != 0 {
		t.Fatal("the redirect was followed")
	}
}

func TestTheRefusalsAreToldApart(t *testing.T) {
	cases := map[string]struct {
		handler http.HandlerFunc
		want    error
	}{
		"used or expired": {func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) }, ErrNotFound},
		"a format this service does not read": {func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write([]byte("%PDF"))
		}, ErrUnsupported},
		"pptx is sent, not received": {func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", MediaType(FormatPPTX))
			_, _ = w.Write([]byte("PK"))
		}, ErrUnsupported},
		"no content type": {func(w http.ResponseWriter, r *http.Request) {
			w.Header()["Content-Type"] = nil
			_, _ = w.Write([]byte("x"))
		}, ErrUnsupported},
		"declared too large": {func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Length", "26214401")
			w.WriteHeader(http.StatusOK)
		}, ErrTooLarge},
		"streamed too large": {func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			chunk := strings.Repeat("a", 1<<20)
			for i := 0; i <= 25; i++ {
				if _, err := w.Write([]byte(chunk)); err != nil {
					return
				}
				w.(http.Flusher).Flush()
			}
		}, ErrTooLarge},
		"server error": {func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) }, ErrUnreachable},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			server, config, _ := peerServing(t, tc.handler)
			_, err := Fetch(context.Background(), NewClient(nil), config, server.URL, claim)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestASlowPeerIsGivenUpOn(t *testing.T) {
	server, config, _ := peerServing(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	})
	client := NewClient(nil)
	client.Timeout = 50 * time.Millisecond
	started := time.Now()
	_, err := Fetch(context.Background(), client, config, server.URL, claim)
	if !errors.Is(err, ErrUnreachable) || time.Since(started) > 2*time.Second {
		t.Fatalf("err = %v after %s", err, time.Since(started))
	}
}

func TestTheDispositionCarriesAKoreanNameWhole(t *testing.T) {
	header := Disposition("2026년 3분기 개편안 (v2); 최종.pptx")
	if strings.ContainsAny(header, ";\"") && strings.Count(header, ";") != 1 {
		t.Fatalf("header = %s", header)
	}
	_, params, err := mime.ParseMediaType(header)
	if err != nil {
		t.Fatal(err)
	}
	if params["filename"] != "2026년 3분기 개편안 (v2); 최종.pptx" {
		t.Fatalf("filename = %q", params["filename"])
	}
	if _, ok := NormalizeFormat(" PPTX "); !ok {
		t.Fatal("pptx is what this service sends")
	}
	if _, ok := NormalizeFormat("markdown"); ok {
		t.Fatal("markdown is received, not sent")
	}
}
