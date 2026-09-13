package analytics

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"
)

func stored(values map[string]string) valueReader {
	reader := valueReader{}
	for key, value := range values {
		reader[key] = json.RawMessage(value)
	}
	return reader
}

func TestNothingIsReadAsOffUntilTheAdministratorTurnsItOn(t *testing.T) {
	config := Read(context.Background(), nil)
	if config.Enabled || config.Provider != ProviderNone || config.Placement != PlacementHead || !config.MomentoProxy {
		t.Fatalf("shipped config = %+v", config)
	}
	if config.Active("/dashboard") {
		t.Fatal("tracking is active on a fresh installation")
	}
	// Every value stored but the switch: still nothing on the page.
	config = Read(context.Background(), stored(map[string]string{
		"analytics.enabled": `false`, "analytics.provider": `"momento"`,
		"analytics.momento_url": `"https://momento.corp.example"`, "analytics.momento_site_id": `"ptium"`,
	}))
	if config.Active("/dashboard") || config.Snippet("n") == "" {
		t.Fatalf("off with a full configuration: active=%v snippet=%q", config.Active("/dashboard"), config.Snippet("n"))
	}
}

func TestAValueThatCannotBeReadIsLeftAtWhatShips(t *testing.T) {
	config := Read(context.Background(), stored(map[string]string{
		"analytics.enabled": `"yes"`, "analytics.provider": `"MOMENTO"`, "analytics.placement": `"footer"`,
		"analytics.momento_proxy": `"no"`,
	}))
	if config.Enabled || config.Provider != ProviderMomento || config.Placement != PlacementHead || !config.MomentoProxy {
		t.Fatalf("config = %+v", config)
	}
}

func TestAdministrativeScreensAreLeftOutUnlessAskedFor(t *testing.T) {
	config := Config{Enabled: true, Provider: ProviderGA4, MeasurementID: "G-1"}
	for path, want := range map[string]bool{"/": true, "/dashboard": true, "/admin": false, "/admin/settings": false, "/administer": true} {
		if got := config.Active(path); got != want {
			t.Errorf("Active(%q) = %v, want %v", path, got, want)
		}
	}
	config.IncludeAdmin = true
	if !config.Active("/admin/settings") {
		t.Fatal("include_admin did not include the admin screen")
	}
}

func TestMomentoIsServedThroughThisOriginByDefault(t *testing.T) {
	config := Config{Enabled: true, Provider: ProviderMomento, MomentoURL: "https://momento.corp.example/", MomentoSiteID: "ptium", MomentoProxy: true}
	snippet := config.Snippet("n0nce")
	for _, want := range []string{`src="/momento/tracker.js"`, `data-endpoint="/momento"`, `data-site-id="ptium"`, `nonce="n0nce"`, `data-contract-version="1"`} {
		if !strings.Contains(snippet, want) {
			t.Errorf("snippet lacks %s: %s", want, snippet)
		}
	}
	if strings.Contains(snippet, "momento.corp.example") {
		t.Fatalf("the collector's address reached the browser: %s", snippet)
	}
	scripts, connects, images := config.PolicySources()
	if len(scripts)+len(connects)+len(images) != 0 {
		t.Fatalf("a proxied collector needs no policy source, got %v %v %v", scripts, connects, images)
	}
	if target := config.ProxyTarget(); target == nil || target.String() != "https://momento.corp.example" {
		t.Fatalf("proxy target = %v", target)
	}

	config.MomentoProxy = false
	snippet = config.Snippet("n0nce")
	if !strings.Contains(snippet, `src="https://momento.corp.example/tracker.js"`) || strings.Contains(snippet, "data-endpoint") {
		t.Fatalf("direct snippet = %s", snippet)
	}
	scripts, _, _ = config.PolicySources()
	if len(scripts) != 1 || scripts[0] != "https://momento.corp.example" {
		t.Fatalf("direct policy sources = %v", scripts)
	}
	if config.ProxyTarget() != nil {
		t.Fatal("the proxy forwards while the browser is told to go direct")
	}
	config.Enabled = false
	config.MomentoProxy = true
	if config.ProxyTarget() != nil {
		t.Fatal("the proxy forwards while tracking is off")
	}
}

func TestEveryScriptTagCarriesTheNonce(t *testing.T) {
	config := Config{Enabled: true, Provider: ProviderCustom, CustomSnippet: `<SCRIPT src="https://t.example/a.js"></SCRIPT><script nonce="mine">x()</script><script>y()</script>`}
	snippet := config.Snippet("abc")
	if got := strings.Count(snippet, `nonce="abc"`); got != 2 {
		t.Fatalf("nonce on %d tags, want 2: %s", got, snippet)
	}
	if !strings.Contains(snippet, `nonce="mine"`) {
		t.Fatalf("a tag's own nonce was replaced: %s", snippet)
	}
	if strings.Contains(snippet, `<sc nonce`) {
		t.Fatalf("the nonce landed inside the tag name: %s", snippet)
	}
	if got := config.Snippet(""); got != strings.TrimSpace(config.CustomSnippet) {
		t.Fatalf("without a nonce the snippet changed: %s", got)
	}
	for _, provider := range []Config{
		{Provider: ProviderGA4, MeasurementID: "G-1"},
		{Provider: ProviderGTM, MeasurementID: "GTM-1"},
		{Provider: ProviderMatomo, MatomoURL: "https://matomo.example", MatomoSiteID: "3"},
		{Provider: ProviderMomento, MomentoURL: "https://momento.example", MomentoSiteID: "s"},
	} {
		rendered := provider.Snippet("z")
		if strings.Count(rendered, "<script") != strings.Count(rendered, `nonce="z"`) {
			t.Errorf("%s: %d tags, %d nonces: %s", provider.Provider, strings.Count(rendered, "<script"), strings.Count(rendered, `nonce="z"`), rendered)
		}
	}
}

// U+212A KELVIN SIGN folds to a one-byte k, U+0130 İ folds to three bytes;
// an index taken from strings.ToLower lands elsewhere in the original.
func TestLettersThatChangeLengthWhenFoldedDoNotMoveTheNonce(t *testing.T) {
	for _, snippet := range []string{
		"İİİİ<script>1</script>",
		"KK<script src=\"https://t.example/k.js\"></script>",
		"<!-- Kelvin K --><SCRIPT>İ</SCRIPT>",
	} {
		got := withNonce(snippet, "n")
		if !strings.Contains(got, `<script nonce="n"`) && !strings.Contains(got, `<SCRIPT nonce="n"`) {
			t.Errorf("%q → %q", snippet, got)
		}
		if strings.Contains(got, "<sc nonce") || strings.Contains(got, "<scr nonce") {
			t.Errorf("the tag was broken: %q → %q", snippet, got)
		}
	}
	if origins := SnippetOrigins("KK<script src=\"https://t.example/k.js\"></script>"); len(origins) != 1 || origins[0] != "https://t.example" {
		t.Fatalf("origins after a Kelvin sign = %v", origins)
	}
}

func TestOriginsAreReadOutOfAPastedSnippet(t *testing.T) {
	snippet := `<script async src="https://cdn.tracker.example/loader.js?id=1"></script>
<script>fetch('HTTPS://collect.tracker.example/v1/events',{method:'POST'});var img=new Image();img.src="http://pixel.example:8080/p.gif";var again="https://cdn.tracker.example/other.js";var word="httpish";var bad="data:text/html,x"</script>`
	got := SnippetOrigins(snippet)
	want := []string{"https://cdn.tracker.example", "https://collect.tracker.example", "http://pixel.example:8080"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("origins = %v, want %v", got, want)
	}
	config := Config{Provider: ProviderCustom, CustomSnippet: snippet, AllowedHosts: "https://extra.example, https://cdn.tracker.example\nhttps://third.example"}
	scripts, connects, images := config.PolicySources()
	if len(scripts) != 6 || len(connects) != 6 || len(images) != 6 {
		t.Fatalf("policy sources = %v / %v / %v", scripts, connects, images)
	}
	if scripts[5] != "https://third.example" {
		t.Fatalf("allowed hosts were not read line by line: %v", scripts)
	}
}

func TestTheAllowListGrowsWithoutRepeatingItself(t *testing.T) {
	list := AddAllowedHost("", "https://a.example/")
	list = AddAllowedHost(list, "https://b.example")
	list = AddAllowedHost(list, "HTTPS://A.EXAMPLE")
	list = AddAllowedHost(list, "  ")
	if list != "https://a.example, https://b.example" {
		t.Fatalf("list = %q", list)
	}
}

func TestASaveThatWouldPutNothingOnThePageIsRefused(t *testing.T) {
	cases := []struct {
		name   string
		config Config
		ok     bool
	}{
		{"off needs nothing", Config{Provider: ProviderMomento}, true},
		{"none needs nothing", Config{Enabled: true, Provider: ProviderNone}, true},
		{"momento without a site", Config{Enabled: true, Provider: ProviderMomento, MomentoURL: "https://m.example"}, false},
		{"momento with a bad address", Config{Enabled: true, Provider: ProviderMomento, MomentoURL: "momento", MomentoSiteID: "s"}, false},
		{"momento complete", Config{Enabled: true, Provider: ProviderMomento, MomentoURL: "https://m.example", MomentoSiteID: "s"}, true},
		{"ga4 without an id", Config{Enabled: true, Provider: ProviderGA4}, false},
		{"gtm with an id", Config{Enabled: true, Provider: ProviderGTM, MeasurementID: "GTM-1"}, true},
		{"matomo without a site", Config{Enabled: true, Provider: ProviderMatomo, MatomoURL: "https://m.example"}, false},
		{"custom empty", Config{Enabled: true, Provider: ProviderCustom}, false},
		{"custom present", Config{Enabled: true, Provider: ProviderCustom, CustomSnippet: "<script></script>"}, true},
		{"unknown provider", Config{Enabled: true, Provider: "piwik"}, false},
		{"too long even while off", Config{Provider: ProviderCustom, CustomSnippet: strings.Repeat("x", MaxSnippetBytes+1)}, false},
	}
	for _, test := range cases {
		err := test.config.Validate()
		if (err == nil) != test.ok {
			t.Errorf("%s: err = %v, want ok=%v", test.name, err, test.ok)
		}
	}
}

func TestBlockedOriginsAreRememberedOnceEach(t *testing.T) {
	recorder := NewRecorder()
	moment := time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)
	recorder.now = func() time.Time { return moment }
	for i := 0; i < 5; i++ {
		recorder.Record("https://momento.corp.example/collect/v1/events", "connect-src", "/dashboard")
	}
	moment = moment.Add(time.Minute)
	recorder.Record("https://cdn.tracker.example/loader.js", "script-src-elem 'self'", "/presentations")
	recorder.Record("chrome-extension://abc/x.js", "script-src", "/")
	recorder.Record("inline", "script-src", "/")
	recorder.Record("data", "img-src", "/")

	items := recorder.List(Config{})
	if len(items) != 2 {
		t.Fatalf("items = %+v", items)
	}
	if items[0].Origin != "https://cdn.tracker.example" || items[0].Directive != "script-src-elem" || items[0].Count != 1 {
		t.Fatalf("newest first: %+v", items[0])
	}
	if items[1].Origin != "https://momento.corp.example" || items[1].Count != 5 || items[1].Allowed {
		t.Fatalf("repeated origin: %+v", items[1])
	}

	allowed := recorder.List(Config{AllowedHosts: "https://momento.corp.example"})
	if !allowed[1].Allowed || allowed[0].Allowed {
		t.Fatalf("allowed marks = %+v", allowed)
	}
	moment = moment.Add(time.Minute)
	recorder.Record("https://region1.google-analytics.com/g/collect", "connect-src", "/")
	wild := recorder.List(Config{Provider: ProviderGA4})
	if !wild[0].Allowed {
		t.Fatalf("wildcard did not cover a regional host: %+v", wild[0])
	}

	recorder.Forget()
	if len(recorder.List(Config{})) != 0 {
		t.Fatal("Forget kept something")
	}
}

func TestTheRecorderHoldsAHundredOriginsAndDropsTheOldest(t *testing.T) {
	recorder := NewRecorder()
	moment := time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)
	recorder.now = func() time.Time { moment = moment.Add(time.Second); return moment }
	for i := 0; i < MaxViolations+10; i++ {
		recorder.Record("https://host"+strings.Repeat("x", i%7)+string(rune('a'+i%26))+".example:"+strconv.Itoa(i), "script-src", "/")
	}
	items := recorder.List(Config{})
	if len(items) != MaxViolations {
		t.Fatalf("held %d, want %d", len(items), MaxViolations)
	}
	for _, item := range items {
		if strings.HasSuffix(item.Origin, ":0") || strings.HasSuffix(item.Origin, ":9") {
			t.Fatalf("the oldest was kept: %s", item.Origin)
		}
	}
}
