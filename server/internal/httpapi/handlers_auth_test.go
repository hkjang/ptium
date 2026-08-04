package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestTokenExchangeSendsTheClientSecret(t *testing.T) {
	var received struct {
		form     url.Values
		username string
		password string
		hasAuth  bool
	}
	provider := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_ = request.ParseForm()
		received.form = request.PostForm
		received.username, received.password, received.hasAuth = request.BasicAuth()
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"access_token":"provider-token","token_type":"Bearer","expires_in":300,"refresh_token":"must-not-leak"}`))
	}))
	defer provider.Close()

	exchange := &TokenExchange{Endpoint: provider.URL, ClientID: "ptium-web", ClientSecret: "s3cret", Client: provider.Client()}
	result, status, err := exchange.exchange(context.Background(), tokenExchangeRequest{
		Code: "auth-code", RedirectURI: "https://ptium.example.com/auth/callback", CodeVerifier: "verifier",
	})
	if err != nil {
		t.Fatalf("exchange: %v (status %d)", err, status)
	}
	if result.AccessToken != "provider-token" || result.ExpiresIn != 300 {
		t.Fatalf("result = %+v", result)
	}
	if !received.hasAuth || received.username != "ptium-web" || received.password != "s3cret" {
		t.Fatalf("the client secret was not presented: %+v", received)
	}
	for key, want := range map[string]string{
		"grant_type": "authorization_code", "code": "auth-code",
		"redirect_uri": "https://ptium.example.com/auth/callback", "code_verifier": "verifier",
	} {
		if got := received.form.Get(key); got != want {
			t.Fatalf("form[%s] = %q, want %q", key, got, want)
		}
	}
	// The secret must never travel in the body where it would reach logs.
	if received.form.Get("client_secret") != "" {
		t.Fatal("the client secret was placed in the request body")
	}
}

func TestTokenExchangeOmitsAuthForAPublicClient(t *testing.T) {
	var hadAuth bool
	provider := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _, hadAuth = request.BasicAuth()
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"access_token":"public-token"}`))
	}))
	defer provider.Close()
	exchange := &TokenExchange{Endpoint: provider.URL, ClientID: "ptium-web", Client: provider.Client()}
	if _, _, err := exchange.exchange(context.Background(), tokenExchangeRequest{
		Code: "c", RedirectURI: "https://example.com/cb", CodeVerifier: "v"}); err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if hadAuth {
		t.Fatal("a public client must not send basic auth")
	}
}

func TestTokenExchangeReportsProviderRejection(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(`{"error":"invalid_grant","error_description":"code already redeemed"}`))
	}))
	defer provider.Close()
	exchange := &TokenExchange{Endpoint: provider.URL, ClientID: "ptium-web", Client: provider.Client()}
	_, status, err := exchange.exchange(context.Background(), tokenExchangeRequest{
		Code: "c", RedirectURI: "https://example.com/cb", CodeVerifier: "v"})
	if err == nil {
		t.Fatal("a provider rejection must be an error")
	}
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if !strings.Contains(err.Error(), "code already redeemed") {
		t.Fatalf("error = %v, want the provider's description", err)
	}
}

func TestTokenExchangeRequiresAnAccessToken(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"token_type":"Bearer"}`))
	}))
	defer provider.Close()
	exchange := &TokenExchange{Endpoint: provider.URL, ClientID: "ptium-web", Client: provider.Client()}
	if _, _, err := exchange.exchange(context.Background(), tokenExchangeRequest{
		Code: "c", RedirectURI: "https://example.com/cb", CodeVerifier: "v"}); err == nil {
		t.Fatal("a response without an access token must fail")
	}
}

func TestValidatePassword(t *testing.T) {
	cases := map[string]bool{
		"":                              false,
		"short":                         false,
		"elevenchars":                   false,
		"twelve chars!":                 true,
		"a-perfectly-ordinary-password": true,
		"            ":                  false,
		strings.Repeat("x", 257):        false,
		strings.Repeat("x", 256):        true,
	}
	for password, valid := range cases {
		message := validatePassword(password)
		if valid && message != "" {
			t.Fatalf("password of length %d rejected: %s", len(password), message)
		}
		if !valid && message == "" {
			t.Fatalf("password %q should have been rejected", password)
		}
	}
}

func TestLoginLimiterBacksOffAfterRepeatedFailures(t *testing.T) {
	limiter := newLoginLimiter()
	now := time.Now()
	limiter.now = func() time.Time { return now }

	// The first attempts up to the threshold are free.
	for attempt := 0; attempt < limiter.threshold; attempt++ {
		if wait := limiter.retryAfter("10.0.0.1"); wait != 0 {
			t.Fatalf("attempt %d should be allowed, got %s", attempt+1, wait)
		}
		limiter.fail("10.0.0.1")
	}
	if wait := limiter.retryAfter("10.0.0.1"); wait != 0 {
		t.Fatalf("the threshold attempt should still be allowed, got %s", wait)
	}
	limiter.fail("10.0.0.1")
	first := limiter.retryAfter("10.0.0.1")
	if first <= 0 {
		t.Fatal("backoff must start once the threshold is passed")
	}
	limiter.fail("10.0.0.1")
	if second := limiter.retryAfter("10.0.0.1"); second <= first {
		t.Fatalf("backoff must grow: %s then %s", first, second)
	}

	// Another client is unaffected.
	if wait := limiter.retryAfter("10.0.0.2"); wait != 0 {
		t.Fatalf("an unrelated client must not be throttled, got %s", wait)
	}
	// Waiting out the block clears it.
	now = now.Add(limiter.ceiling + time.Second)
	if wait := limiter.retryAfter("10.0.0.1"); wait != 0 {
		t.Fatalf("the block must expire, got %s", wait)
	}
	// A successful sign-in forgets the history.
	limiter.fail("10.0.0.3")
	limiter.succeed("10.0.0.3")
	if _, tracked := limiter.attempts["10.0.0.3"]; tracked {
		t.Fatal("a successful sign-in must clear the client's history")
	}
	// Idle clients are swept.
	now = now.Add(limiter.window + time.Minute)
	limiter.retryAfter("10.0.0.9")
	if len(limiter.attempts) != 0 {
		t.Fatalf("idle clients should have been swept, %d remain", len(limiter.attempts))
	}
	// A nil limiter and empty client are safe.
	var absent *loginLimiter
	if absent.retryAfter("10.0.0.1") != 0 {
		t.Fatal("a nil limiter must allow requests")
	}
	absent.fail("10.0.0.1")
	absent.succeed("10.0.0.1")
}

func TestClientAddressStripsThePort(t *testing.T) {
	cases := map[string]string{
		"10.0.0.5:54321":    "10.0.0.5",
		"[2001:db8::1]:443": "2001:db8::1",
		// An address without a port is used whole, including bare IPv6, which a
		// naive split on the last colon would truncate.
		"192.168.1.1": "192.168.1.1",
		"2001:db8::1": "2001:db8::1",
		"":            "",
	}
	for input, want := range cases {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		request.RemoteAddr = input
		if got := clientAddress(request); got != want {
			t.Fatalf("clientAddress(%q) = %q, want %q", input, got, want)
		}
	}
}
