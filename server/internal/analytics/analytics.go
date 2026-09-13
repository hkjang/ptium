// Package analytics puts a visitor tracking snippet on the served pages.
//
// The workspace ships with a policy that runs scripts from its own origin only,
// so a snippet pasted into the page is refused without a word to the person
// who pasted it. This package produces both halves of the answer: the markup to
// inject, with a per-request nonce on every script tag so the policy stays
// strict, and the origins that policy has to allow for the snippet to work.
package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"strings"
)

const (
	ProviderNone    = "none"
	ProviderMomento = "momento"
	ProviderGA4     = "ga4"
	ProviderGTM     = "gtm"
	ProviderMatomo  = "matomo"
	ProviderCustom  = "custom"

	PlacementHead = "head"
	PlacementBody = "body"

	// MaxSnippetBytes bounds a pasted snippet. A loader is a few hundred bytes;
	// anything larger is a page, not a tracker.
	MaxSnippetBytes = 8 * 1024

	// ProxyPath is where the workspace forwards Momento traffic when the
	// same-origin proxy is on, so the collector never appears in the policy.
	ProxyPath = "/momento"
)

// Providers lists what can be chosen, in the order the screen offers them.
// Momento comes first: it is the self-hosted collector, the one choice whose
// data never leaves the network.
var Providers = []string{ProviderNone, ProviderMomento, ProviderGA4, ProviderGTM, ProviderMatomo, ProviderCustom}

// Placements are the two places a snippet can be put.
var Placements = []string{PlacementHead, PlacementBody}

// Config is the tracking configuration as the administrator stored it.
type Config struct {
	Enabled       bool
	Provider      string
	MomentoURL    string
	MomentoSiteID string
	// MomentoProxy sends the tracker and its events through this origin at
	// ProxyPath rather than straight to the collector.
	MomentoProxy  bool
	MeasurementID string
	MatomoURL     string
	MatomoSiteID  string
	CustomSnippet string
	AllowedHosts  string
	IncludeAdmin  bool
	Placement     string
}

// Reader is the little of the settings service this needs.
type Reader interface {
	Get(ctx context.Context, key string, target any) error
}

// Read maps the stored settings onto the configuration. A value that cannot be
// read is left at what the product ships, so a settings outage never puts a
// snippet on a page or takes one off.
func Read(ctx context.Context, reader Reader) Config {
	config := Config{Provider: ProviderNone, Placement: PlacementHead, MomentoProxy: true}
	if reader == nil {
		return config
	}
	flag := func(key string, target *bool) {
		var value bool
		if reader.Get(ctx, key, &value) == nil {
			*target = value
		}
	}
	word := func(key string, target *string) {
		var value string
		if reader.Get(ctx, key, &value) == nil {
			*target = strings.TrimSpace(value)
		}
	}
	flag("analytics.enabled", &config.Enabled)
	word("analytics.provider", &config.Provider)
	word("analytics.momento_url", &config.MomentoURL)
	word("analytics.momento_site_id", &config.MomentoSiteID)
	flag("analytics.momento_proxy", &config.MomentoProxy)
	word("analytics.measurement_id", &config.MeasurementID)
	word("analytics.matomo_url", &config.MatomoURL)
	word("analytics.matomo_site_id", &config.MatomoSiteID)
	word("analytics.custom_snippet", &config.CustomSnippet)
	word("analytics.allowed_hosts", &config.AllowedHosts)
	flag("analytics.include_admin", &config.IncludeAdmin)
	word("analytics.placement", &config.Placement)
	config.Provider = strings.ToLower(config.Provider)
	if config.Provider == "" {
		config.Provider = ProviderNone
	}
	config.Placement = strings.ToLower(config.Placement)
	if config.Placement != PlacementBody {
		config.Placement = PlacementHead
	}
	return config
}

// FromValues maps stored JSON values onto the configuration, for a caller that
// already holds the settings — the write path, checking what a save would
// leave behind.
func FromValues(values map[string]json.RawMessage) Config {
	return Read(context.Background(), valueReader(values))
}

type valueReader map[string]json.RawMessage

func (v valueReader) Get(_ context.Context, key string, target any) error {
	raw, ok := v[key]
	if !ok {
		return fmt.Errorf("setting %q is not set", key)
	}
	return json.Unmarshal(raw, target)
}

// Active reports whether a page at this path should carry the snippet. The
// administrative screens are left out unless asked for: console traffic is
// rarely the visitor data anybody wants to count.
func (c Config) Active(path string) bool {
	if !c.Enabled || c.Provider == ProviderNone || c.Provider == "" {
		return false
	}
	if !c.IncludeAdmin && (path == "/admin" || strings.HasPrefix(path, "/admin/")) {
		return false
	}
	return strings.TrimSpace(c.Snippet("")) != ""
}

// Validate reports what is missing for the chosen provider, so a save that
// would put nothing on the page is refused rather than stored.
func (c Config) Validate() error {
	if len(c.CustomSnippet) > MaxSnippetBytes {
		return fmt.Errorf("the tracking snippet cannot be longer than %d bytes", MaxSnippetBytes)
	}
	if !c.Enabled {
		return nil
	}
	switch c.Provider {
	case ProviderNone, "":
		return nil
	case ProviderMomento:
		if strings.TrimSpace(c.MomentoURL) == "" || strings.TrimSpace(c.MomentoSiteID) == "" {
			return fmt.Errorf("analytics.momento_url and analytics.momento_site_id are required for Momento")
		}
		if originOf(c.MomentoURL) == "" {
			return fmt.Errorf("analytics.momento_url must be an HTTP(S) URL")
		}
	case ProviderGA4, ProviderGTM:
		if strings.TrimSpace(c.MeasurementID) == "" {
			return fmt.Errorf("analytics.measurement_id is required for %s", c.Provider)
		}
	case ProviderMatomo:
		if strings.TrimSpace(c.MatomoURL) == "" || strings.TrimSpace(c.MatomoSiteID) == "" {
			return fmt.Errorf("analytics.matomo_url and analytics.matomo_site_id are required for Matomo")
		}
		if originOf(c.MatomoURL) == "" {
			return fmt.Errorf("analytics.matomo_url must be an HTTP(S) URL")
		}
	case ProviderCustom:
		if strings.TrimSpace(c.CustomSnippet) == "" {
			return fmt.Errorf("analytics.custom_snippet is empty")
		}
	default:
		return fmt.Errorf("analytics.provider must be one of %s", strings.Join(Providers, ", "))
	}
	return nil
}

// Snippet renders the markup to inject. The nonce goes on every script tag so
// the page's policy can stay strict.
func (c Config) Snippet(nonce string) string {
	switch c.Provider {
	case ProviderMomento:
		site := html.EscapeString(strings.TrimSpace(c.MomentoSiteID))
		base := strings.TrimRight(strings.TrimSpace(c.MomentoURL), "/")
		if site == "" || base == "" {
			return ""
		}
		if c.MomentoProxy {
			// Through this origin: the tracker is fetched from and reports to
			// ProxyPath, and the collector's address never reaches the browser.
			return withNonce(fmt.Sprintf(`<script async src="%s/tracker.js" data-site-id="%s" data-environment="prd" data-contract-version="1" data-endpoint="%s"></script>`,
				ProxyPath, site, ProxyPath), nonce)
		}
		return withNonce(fmt.Sprintf(`<script async src="%s/tracker.js" data-site-id="%s" data-environment="prd" data-contract-version="1"></script>`,
			html.EscapeString(base), site), nonce)
	case ProviderGA4:
		id := html.EscapeString(strings.TrimSpace(c.MeasurementID))
		if id == "" {
			return ""
		}
		return withNonce(fmt.Sprintf(`<script async src="https://www.googletagmanager.com/gtag/js?id=%s"></script>
<script>window.dataLayer=window.dataLayer||[];function gtag(){dataLayer.push(arguments);}gtag('js',new Date());gtag('config','%s');</script>`, id, id), nonce)
	case ProviderGTM:
		id := html.EscapeString(strings.TrimSpace(c.MeasurementID))
		if id == "" {
			return ""
		}
		return withNonce(fmt.Sprintf(`<script>(function(w,d,s,l,i){w[l]=w[l]||[];w[l].push({'gtm.start':new Date().getTime(),event:'gtm.js'});var f=d.getElementsByTagName(s)[0],j=d.createElement(s),dl=l!='dataLayer'?'&l='+l:'';j.async=true;j.src='https://www.googletagmanager.com/gtm.js?id='+i+dl;f.parentNode.insertBefore(j,f);})(window,document,'script','dataLayer','%s');</script>`, id), nonce)
	case ProviderMatomo:
		base := strings.TrimRight(strings.TrimSpace(c.MatomoURL), "/")
		site := html.EscapeString(strings.TrimSpace(c.MatomoSiteID))
		if base == "" || site == "" {
			return ""
		}
		return withNonce(fmt.Sprintf(`<script>var _paq=window._paq=window._paq||[];_paq.push(['trackPageView']);_paq.push(['enableLinkTracking']);(function(){var u="%s/";_paq.push(['setTrackerUrl',u+'matomo.php']);_paq.push(['setSiteId','%s']);var d=document,g=d.createElement('script'),s=d.getElementsByTagName('script')[0];g.async=true;g.src=u+'matomo.js';s.parentNode.insertBefore(g,s);})();</script>`, html.EscapeString(base), site), nonce)
	case ProviderCustom:
		return withNonce(strings.TrimSpace(c.CustomSnippet), nonce)
	}
	return ""
}

// ProxyTarget is the collector Momento traffic at ProxyPath is forwarded to,
// or nil when nothing should be forwarded: tracking off, another provider, or
// the proxy turned off so the browser talks to the collector itself.
func (c Config) ProxyTarget() *url.URL {
	if !c.Enabled || c.Provider != ProviderMomento || !c.MomentoProxy {
		return nil
	}
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(c.MomentoURL), "/"))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil
	}
	return parsed
}

// PolicySources lists the origins the snippet needs, derived from the provider
// so a common setup needs no policy knowledge at all.
func (c Config) PolicySources() (scripts []string, connects []string, images []string) {
	add := func(origin string) {
		scripts = append(scripts, origin)
		connects = append(connects, origin)
		images = append(images, origin)
	}
	switch c.Provider {
	case ProviderMomento:
		// Through the proxy, the collector is this origin, which 'self' covers.
		if !c.MomentoProxy {
			if origin := originOf(c.MomentoURL); origin != "" {
				add(origin)
			}
		}
	case ProviderGA4, ProviderGTM:
		scripts = append(scripts, "https://www.googletagmanager.com")
		connects = append(connects, "https://www.google-analytics.com", "https://analytics.google.com", "https://*.google-analytics.com")
		images = append(images, "https://www.google-analytics.com", "https://www.googletagmanager.com")
	case ProviderMatomo:
		if origin := originOf(c.MatomoURL); origin != "" {
			add(origin)
		}
	case ProviderCustom:
		// A pasted snippet names the addresses it loads from and reports to,
		// so those are allowed without anybody reading a policy error first.
		for _, origin := range SnippetOrigins(c.CustomSnippet) {
			add(origin)
		}
	}
	for _, host := range SplitHosts(c.AllowedHosts) {
		add(host)
	}
	return scripts, connects, images
}

// SplitHosts reads the administrator's allow list, one origin per comma,
// space or line.
func SplitHosts(list string) []string {
	var hosts []string
	for _, host := range strings.FieldsFunc(list, func(letter rune) bool {
		return letter == ',' || letter == ' ' || letter == '\n' || letter == '\r' || letter == '\t'
	}) {
		if trimmed := strings.TrimSpace(host); trimmed != "" {
			hosts = append(hosts, trimmed)
		}
	}
	return hosts
}

// AddAllowedHost appends an origin to the allow list, leaving the existing
// entries and their order alone. It is what the one-click fix writes.
func AddAllowedHost(existing, origin string) string {
	origin = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(origin), "/"))
	if origin == "" {
		return existing
	}
	for _, host := range SplitHosts(existing) {
		if strings.EqualFold(host, origin) {
			return existing
		}
	}
	if strings.TrimSpace(existing) == "" {
		return origin
	}
	return strings.TrimSpace(existing) + ", " + origin
}

// SnippetOrigins lists every http(s) origin written into a snippet: the script
// it loads, the endpoint it posts to, the pixel it requests. A tracker almost
// always writes its own address into its loader, so reading them here is what
// keeps a pasted snippet working without the administrator translating a
// policy error into a host name.
func SnippetOrigins(snippet string) []string {
	origins := make([]string, 0, 2)
	seen := make(map[string]struct{}, 2)
	for index := 0; index < len(snippet); {
		start := indexFold(snippet[index:], "http")
		if start < 0 {
			break
		}
		start += index
		end := start
		for end < len(snippet) && !isURLBoundary(snippet[end]) {
			end++
		}
		index = end
		origin := originOf(snippet[start:end])
		if origin == "" {
			continue
		}
		if _, duplicate := seen[origin]; duplicate {
			continue
		}
		seen[origin] = struct{}{}
		origins = append(origins, origin)
	}
	return origins
}

// isURLBoundary reports the characters that cannot appear in a URL written
// inside HTML or JavaScript, which is where each address ends.
func isURLBoundary(letter byte) bool {
	switch letter {
	case '"', '\'', '`', '<', '>', ' ', '\t', '\n', '\r', ')', ',', ';', '\\', '+':
		return true
	}
	return false
}

// originOf is the scheme and host of an http(s) address, or "" for anything
// else: a data: URL, a browser extension, a bare word that merely starts with
// "http".
func originOf(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return ""
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return ""
	}
	return scheme + "://" + strings.ToLower(parsed.Host)
}

// withNonce adds the nonce to every script tag that does not already carry
// one, which is what lets a pasted snippet run under a strict policy unchanged.
func withNonce(snippet, nonce string) string {
	if nonce == "" || snippet == "" {
		return snippet
	}
	var builder strings.Builder
	remaining := snippet
	for {
		index := indexFold(remaining, "<script")
		if index < 0 {
			builder.WriteString(remaining)
			return builder.String()
		}
		end := index + len("<script")
		builder.WriteString(remaining[:end])
		tag := remaining[end:]
		if closing := strings.IndexByte(tag, '>'); closing >= 0 {
			tag = tag[:closing]
		}
		if !containsFold(tag, "nonce=") {
			builder.WriteString(` nonce="` + html.EscapeString(nonce) + `"`)
		}
		remaining = remaining[end:]
	}
}

// indexFold finds sub in s ignoring ASCII case, and returns an index into s.
//
// strings.ToLower is the obvious way and the wrong one: it changes byte
// lengths for some runes — U+212A KELVIN SIGN is three bytes and folds to a
// one-byte 'k', U+0130 'İ' is two and folds to three — so an index taken from
// the folded copy lands somewhere else in the original. One such letter in a
// snippet put the nonce into the middle of the tag name, and the tracking
// stopped without saying so. Every needle here is ASCII, and folding only
// ASCII keeps every byte where it was.
func indexFold(s, sub string) int {
	if len(sub) == 0 {
		return 0
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		match := true
		for j := 0; j < len(sub); j++ {
			if foldASCII(s[i+j]) != foldASCII(sub[j]) {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// containsFold reports whether sub appears in s, ignoring ASCII case.
func containsFold(s, sub string) bool { return indexFold(s, sub) >= 0 }

func foldASCII(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}
