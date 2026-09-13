package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hkjang/ptium/server/internal/handoff"
)

func handoffServer(entries ...string) *Server {
	config, err := handoff.ParsePeers(entries)
	if err != nil {
		panic(err)
	}
	return &Server{
		logger:        slog.New(slog.DiscardHandler),
		readPeers:     func(context.Context) handoff.Config { return config },
		handoffClient: handoff.NewClient(nil),
	}
}

func TestTheSourceAPeerIsToldIsThePublicAddressOrTheOneAsked(t *testing.T) {
	server := handoffServer()
	plain := httptest.NewRequest(http.MethodGet, "/api/v1/handoff/targets", nil)
	plain.Host = "slides.intra:8080"
	if got := server.publicOrigin(plain); got != "http://slides.intra:8080" {
		t.Fatalf("origin = %q", got)
	}
	proxied := httptest.NewRequest(http.MethodGet, "/api/v1/handoff/targets", nil)
	proxied.Host = "10.0.0.5:8080"
	proxied.Header.Set("X-Forwarded-Proto", "https")
	proxied.Header.Set("X-Forwarded-Host", "Slides.Corp.Example, 10.0.0.5:8080")
	if got := server.publicOrigin(proxied); got != "https://slides.corp.example" {
		t.Fatalf("origin behind a proxy = %q", got)
	}
	server.publicBaseURL = "https://ptium.intra/base/"
	if got := server.publicOrigin(proxied); got != "https://ptium.intra" {
		t.Fatalf("origin with PUBLIC_BASE_URL = %q", got)
	}
}

func TestTheDestinationsAreEmptyUntilAnAdministratorNamesOneThatTakesADeck(t *testing.T) {
	read := func(server *Server) (string, []handoff.Target) {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/v1/handoff/targets", nil)
		request.Host = "slides.intra"
		server.handoffTargets(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d", recorder.Code)
		}
		var body struct {
			Data struct {
				Source  string           `json:"source"`
				Targets []handoff.Target `json:"targets"`
			} `json:"data"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.Data.Source, body.Data.Targets
	}
	// As shipped, and with only services that do not receive a presentation.
	for _, server := range []*Server{handoffServer(), handoffServer("umm=https://umm.intra", "muni=https://muni.intra", "kanpic=https://kanpic.intra")} {
		if source, targets := read(server); source != "http://slides.intra" || len(targets) != 0 {
			t.Fatalf("source = %q targets = %+v", source, targets)
		}
	}
	_, targets := read(handoffServer("umm=https://umm.intra", "weekly=https://weekly.intra"))
	if len(targets) != 1 || targets[0].Name != "weekly" || targets[0].Origin != "https://weekly.intra" || strings.Join(targets[0].Formats, ",") != "pptx" {
		t.Fatalf("targets = %+v", targets)
	}
	// And the answer is JSON the list is read from, not the empty-slice nil.
	recorder := httptest.NewRecorder()
	handoffServer().handoffTargets(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/handoff/targets", nil))
	if !strings.Contains(recorder.Body.String(), `"targets":[]`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestASourceOffTheListIsRefusedBeforeAnyRequestIsMade(t *testing.T) {
	var hits atomic.Int32
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer peer.Close()
	receive := func(server *Server, source string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		body := `{"source":` + strconvQuote(source) + `,"claim":"abcdefghijklmnopqrstuvwxyz012345"}`
		request := httptest.NewRequest(http.MethodPost, "/api/v1/handoff/receive", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		server.receiveHandoff(recorder, request)
		return recorder
	}
	for name, server := range map[string]*Server{
		"an empty list":                 handoffServer(),
		"a list naming another service": handoffServer("muni=https://muni.intra"),
	} {
		recorder := receive(server, peer.URL)
		if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "handoff_source_not_allowed") {
			t.Fatalf("%s: %d %s", name, recorder.Code, recorder.Body.String())
		}
	}
	// On the list, but with a path on the source: not an origin, not asked.
	listed := handoffServer("umm=" + peer.URL)
	if recorder := receive(listed, peer.URL+"/anything"); recorder.Code != http.StatusForbidden {
		t.Fatalf("a source with a path answered %d", recorder.Code)
	}
	if hits.Load() != 0 {
		t.Fatalf("the peer was asked %d times", hits.Load())
	}
	// On the list: the peer is asked, and what it answered with — nothing,
	// of no type — is refused in words.
	recorder := receive(listed, peer.URL)
	if recorder.Code != http.StatusUnsupportedMediaType || !strings.Contains(recorder.Body.String(), "handoff_unsupported_format") {
		t.Fatalf("an empty 200 from the peer answered %d %s", recorder.Code, recorder.Body.String())
	}
	if hits.Load() != 1 {
		t.Fatalf("the listed peer was asked %d times", hits.Load())
	}
}

func TestEachWayAFetchFailsIsToldApartInTheAnswer(t *testing.T) {
	for name, tc := range map[string]struct {
		handler http.HandlerFunc
		status  int
		code    string
	}{
		"used or expired": {func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) }, http.StatusNotFound, "handoff_claim_refused"},
		"redirect": {func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "https://elsewhere.example/", http.StatusFound)
		}, http.StatusBadGateway, "handoff_redirected"},
		"a pdf": {func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write([]byte("%PDF"))
		}, http.StatusUnsupportedMediaType, "handoff_unsupported_format"},
		"declared too big": {func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Length", "99999999")
			w.WriteHeader(200)
		}, http.StatusRequestEntityTooLarge, "handoff_too_large"},
	} {
		t.Run(name, func(t *testing.T) {
			peer := httptest.NewServer(tc.handler)
			defer peer.Close()
			server := handoffServer("umm=" + peer.URL)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/v1/handoff/receive",
				strings.NewReader(`{"source":"`+peer.URL+`","claim":"abcdefghijklmnopqrstuvwxyz012345"}`))
			request.Header.Set("Content-Type", "application/json")
			server.receiveHandoff(recorder, request)
			if recorder.Code != tc.status || !strings.Contains(recorder.Body.String(), tc.code) {
				t.Fatalf("%d %s", recorder.Code, recorder.Body.String())
			}
		})
	}
	// A claim that is not a claim is refused without asking either.
	server := handoffServer("umm=https://umm.intra")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/handoff/receive",
		strings.NewReader(`{"source":"https://umm.intra","claim":"../x"}`))
	request.Header.Set("Content-Type", "application/json")
	server.receiveHandoff(recorder, request)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "handoff_claim_invalid") {
		t.Fatalf("%d %s", recorder.Code, recorder.Body.String())
	}
}

func TestAServedClaimNeverReachesTheLog(t *testing.T) {
	for path, want := range map[string]string{
		"/api/v1/handoff/claims/abcdefghijklmnopqrstuvwxyz012345": "/api/v1/handoff/claims/{claim}",
		"/api/v1/handoff/claims/":                                 "/api/v1/handoff/claims/",
		"/api/v1/handoff/claims":                                  "/api/v1/handoff/claims",
		"/api/v1/handoff/targets":                                 "/api/v1/handoff/targets",
		"/api/v1/presentations/abc/export":                        "/api/v1/presentations/abc/export",
	} {
		if got := loggedPath(path); got != want {
			t.Errorf("loggedPath(%q) = %q", path, got)
		}
	}
	// And the request log goes through it.
	var lines strings.Builder
	server := handoffServer()
	server.logger = slog.New(slog.NewTextHandler(&lines, nil))
	handler := server.requestMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) }))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/handoff/claims/SECRETSECRETSECRETSECRET", nil))
	if strings.Contains(lines.String(), "SECRETSECRET") || !strings.Contains(lines.String(), "{claim}") {
		t.Fatalf("log = %s", lines.String())
	}
}

func TestAMalformedPeerListIsNotStoredAndAStoredOneIsRead(t *testing.T) {
	for _, good := range []string{`[]`, `["weekly=https://weekly.intra"]`, `["umm=http://umm.intra:8080","muni=https://muni.intra"]`} {
		if err := validateSettingValue(handoff.SettingKey, json.RawMessage(good)); err != nil {
			t.Errorf("%s refused: %v", good, err)
		}
	}
	for _, bad := range []string{`"weekly=https://weekly.intra"`, `["https://weekly.intra"]`, `["weekly=https://weekly.intra/handoff"]`, `["weekly=https://a.intra","muni=https://a.intra"]`, `[` + strings.Repeat(`"x=https://x.intra",`, 50) + `"y=https://y.intra"]`} {
		if err := validateSettingValue(handoff.SettingKey, json.RawMessage(bad)); err == nil {
			t.Errorf("%s was accepted", bad)
		}
	}
	// What is stored is what is read; a row that stopped parsing is nobody.
	stored := valueReader{handoff.SettingKey: json.RawMessage(`["weekly=https://weekly.intra"]`)}
	if got := peersFrom(context.Background(), stored).Targets(); len(got) != 1 {
		t.Fatalf("targets = %+v", got)
	}
	broken := valueReader{handoff.SettingKey: json.RawMessage(`["weekly=https://weekly.intra","nonsense"]`)}
	if got := peersFrom(context.Background(), broken).Peers; len(got) != 0 {
		t.Fatalf("a broken list yielded %+v", got)
	}
	if got := peersFrom(context.Background(), nil).Peers; len(got) != 0 {
		t.Fatalf("no reader yielded %+v", got)
	}
}

type valueReader map[string]json.RawMessage

func (v valueReader) Get(_ context.Context, key string, target any) error {
	raw, ok := v[key]
	if !ok {
		return context.Canceled
	}
	return json.Unmarshal(raw, target)
}

func strconvQuote(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
