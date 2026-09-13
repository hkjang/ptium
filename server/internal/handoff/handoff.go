// Package handoff passes a document between this service and another one on
// the same network without anybody downloading a file.
//
// The shape is the one every service here follows (HANDOFF-STANDARD): the
// sender issues a claim — a short, single-use token bound to one document —
// and the receiver, given the sender's origin and the claim, fetches the
// document from the sender itself. No service holds another's credentials.
//
// This is the part that touches no database: Ptium's row in the standard's
// format table, the administrator's list of peers, and the fetch on the
// receiving side with every guard the standard demands — because the source
// is a value that came in from outside, and fetched as it came it would make
// this service a tool for reading any address on the network.
package handoff

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"
	"time"
)

const (
	FormatMarkdown = "markdown"
	FormatDOCX     = "docx"
	FormatCSV      = "csv"
	FormatXLSX     = "xlsx"
	FormatTXT      = "txt"
	FormatPPTX     = "pptx"

	// ClaimTTL is how long a claim can be redeemed. The standard says five
	// minutes at most; the token is the only credential, so it stays short.
	ClaimTTL = 5 * time.Minute

	// MaxBodyBytes and FetchTimeout bound what the receiving side takes from
	// a peer, however trusted the list says it is.
	MaxBodyBytes = 25 << 20
	FetchTimeout = 30 * time.Second

	// ClaimsPath is the sender's endpoint; the receiver appends the claim.
	ClaimsPath = "/api/v1/handoff/claims"
	// ReceivePath is where a browser brings a claim to the receiving side.
	ReceivePath = "/handoff"

	// SettingKey holds the administrator's peer list.
	SettingKey = "handoff.peers"
)

// Sends and Receives are Ptium's row in the standard's format table: a deck
// goes out as a presentation, and what comes in is the material a deck is
// made from.
var (
	Sends    = []string{FormatPPTX}
	Receives = []string{FormatMarkdown, FormatDOCX, FormatCSV, FormatXLSX, FormatTXT}
)

// table is the standard's format table — what each service on the network
// receives — so a peer named after one of them needs no more than its
// address. A destination is shown only for a format it can take.
var table = map[string][]string{
	"umm":    {},
	"muni":   {FormatMarkdown},
	"kanpic": {FormatCSV, FormatXLSX},
	"ptium":  Receives,
	"weekly": {FormatMarkdown, FormatDOCX, FormatPPTX},
}

// mediaTypes is what a format arrives as. The check is on the parsed media
// type, so a charset parameter neither helps nor hurts.
var mediaTypes = map[string][]string{
	FormatMarkdown: {"text/markdown", "text/x-markdown"},
	FormatDOCX:     {"application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
	FormatCSV:      {"text/csv"},
	FormatXLSX:     {"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
	FormatTXT:      {"text/plain"},
	FormatPPTX:     {"application/vnd.openxmlformats-officedocument.presentationml.presentation"},
}

// extensions is the file suffix a received format is read under, which is how
// the document reader tells them apart.
var extensions = map[string]string{
	FormatMarkdown: ".md",
	FormatDOCX:     ".docx",
	FormatCSV:      ".csv",
	FormatXLSX:     ".xlsx",
	FormatTXT:      ".txt",
	FormatPPTX:     ".pptx",
}

// Peer is one service the administrator allowed: what to call it, where it
// lives, and which formats it takes.
type Peer struct {
	Name     string   `json:"name"`
	Origin   string   `json:"origin"`
	Receives []string `json:"receives"`
}

// Config is the allow list. It is empty as shipped, and while it is empty
// nothing is sent and nothing is accepted: a fresh install is unchanged.
type Config struct {
	Peers []Peer
}

// ParsePeers reads the stored list. Each entry is `name=origin`; the name is
// one of the services in the standard's table, which says what it receives,
// or any other word for a peer this service only receives from.
func ParsePeers(entries []string) (Config, error) {
	config := Config{}
	seen := map[string]bool{}
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		name, address, found := strings.Cut(entry, "=")
		name = strings.ToLower(strings.TrimSpace(name))
		if !found || name == "" {
			return Config{}, fmt.Errorf("%q: 각 줄은 이름=오리진 꼴이어야 합니다 (예: weekly=https://weekly.intra)", entry)
		}
		if strings.ContainsAny(name, " \t") || len([]rune(name)) > 60 {
			return Config{}, fmt.Errorf("%q: 서비스 이름은 공백 없이 60자 이하여야 합니다", entry)
		}
		origin := strictOrigin(address)
		if origin == "" {
			return Config{}, fmt.Errorf("%q: 주소는 경로 없는 http 또는 https 오리진이어야 합니다", entry)
		}
		if seen[origin] {
			return Config{}, fmt.Errorf("같은 주소가 두 번 있습니다: %s", origin)
		}
		seen[origin] = true
		receives, known := table[name]
		if !known {
			receives = []string{}
		}
		config.Peers = append(config.Peers, Peer{Name: name, Origin: origin, Receives: slices.Clone(receives)})
	}
	return config, nil
}

// Allowed reports whether a source a browser brought is on the list. The
// comparison is on the whole origin; a path, query or userinfo on the source
// is not an origin and matches nothing.
func (c Config) Allowed(source string) (Peer, bool) {
	origin := strictOrigin(source)
	if origin == "" {
		return Peer{}, false
	}
	for _, peer := range c.Peers {
		if peer.Origin == origin {
			return peer, true
		}
	}
	return Peer{}, false
}

// Target is a peer as the send menu sees it: only the formats this service
// sends and the peer receives.
type Target struct {
	Name    string   `json:"name"`
	Origin  string   `json:"origin"`
	Formats []string `json:"formats"`
}

// Targets lists where a deck can go. A peer that receives nothing this
// service sends is not a destination and does not appear, so an empty answer
// is what hides the button.
func (c Config) Targets() []Target {
	targets := []Target{}
	for _, peer := range c.Peers {
		formats := []string{}
		for _, format := range Sends {
			if slices.Contains(peer.Receives, format) {
				formats = append(formats, format)
			}
		}
		if len(formats) == 0 {
			continue
		}
		targets = append(targets, Target{Name: peer.Name, Origin: peer.Origin, Formats: formats})
	}
	return targets
}

// ReceiveURL is where the sender's browser opens the receiving side.
func ReceiveURL(target, source, claim string) string {
	query := url.Values{}
	query.Set("source", source)
	query.Set("claim", claim)
	return target + ReceivePath + "?" + query.Encode()
}

// OriginOf reduces an address to its origin: scheme and host, lowered.
// Anything that is not an http(s) address is "".
func OriginOf(raw string) string {
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

// strictOrigin is OriginOf for a value that must already be an origin. A
// source with a path would let "https://umm.intra/../../other" claim to be
// umm; it is refused rather than trimmed.
func strictOrigin(raw string) string {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
		return ""
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return ""
	}
	return OriginOf(raw)
}

// NewClaim is a fresh token: 32 random bytes, which is twice the 128 bits the
// standard asks for, in the alphabet ValidClaim accepts.
func NewClaim() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// ValidClaim reports whether a claim looks like one a peer would have issued:
// letters, digits and the two URL-safe marks, and nothing that could be a
// path.
func ValidClaim(claim string) bool {
	if len(claim) < 16 || len(claim) > 256 {
		return false
	}
	for _, letter := range claim {
		switch {
		case letter >= 'a' && letter <= 'z', letter >= 'A' && letter <= 'Z', letter >= '0' && letter <= '9', letter == '-', letter == '_':
		default:
			return false
		}
	}
	return true
}

// Document is what a claim turned out to be.
type Document struct {
	Format string
	// Filename is what the sender called it, with the format's suffix on it
	// even when the sender said no name, so a reader can tell what it is.
	Filename    string
	ContentType string
	Body        []byte
}

// The ways a fetch fails, each of which the receiving page says in words.
var (
	ErrNotAllowed  = errors.New("source is not on the allow list")
	ErrBadClaim    = errors.New("claim is malformed")
	ErrNotFound    = errors.New("claim was refused by the source")
	ErrRedirect    = errors.New("source answered with a redirect")
	ErrUnsupported = errors.New("source sent a format this service cannot read")
	ErrTooLarge    = errors.New("document is larger than the limit")
	ErrUnreachable = errors.New("source could not be reached")
)

// NewClient is the client every fetch goes through: it never follows a
// redirect — a peer that redirects is sending the request somewhere the list
// did not approve — and it gives up after FetchTimeout.
func NewClient(transport http.RoundTripper) *http.Client {
	return &http.Client{
		Transport: transport,
		Timeout:   FetchTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// Fetch redeems a claim at a source. The allow list is checked first, and a
// source that is not on it costs no request at all. The response is refused
// before its body is read when its type is not one this service receives,
// and the body is cut off at MaxBodyBytes.
func Fetch(ctx context.Context, client *http.Client, config Config, source, claim string) (Document, error) {
	peer, ok := config.Allowed(source)
	if !ok {
		return Document{}, ErrNotAllowed
	}
	if !ValidClaim(claim) {
		return Document{}, ErrBadClaim
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, peer.Origin+ClaimsPath+"/"+claim, nil)
	if err != nil {
		return Document{}, ErrUnreachable
	}
	request.Header.Set("Accept", strings.Join(accepted(), ", "))
	response, err := client.Do(request)
	if err != nil {
		return Document{}, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	defer response.Body.Close()
	switch {
	case response.StatusCode >= 300 && response.StatusCode < 400:
		return Document{}, ErrRedirect
	case response.StatusCode == http.StatusNotFound:
		return Document{}, ErrNotFound
	case response.StatusCode != http.StatusOK:
		return Document{}, fmt.Errorf("%w: status %d", ErrUnreachable, response.StatusCode)
	}
	format, ok := formatOf(response.Header.Get("Content-Type"))
	if !ok {
		return Document{}, ErrUnsupported
	}
	if response.ContentLength > MaxBodyBytes {
		return Document{}, ErrTooLarge
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, MaxBodyBytes+1))
	if err != nil {
		return Document{}, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	if len(body) > MaxBodyBytes {
		return Document{}, ErrTooLarge
	}
	filename := filenameOf(response.Header.Get("Content-Disposition"))
	if filename == "" {
		filename = "handoff"
	}
	if !strings.EqualFold(path.Ext(filename), extensions[format]) {
		filename += extensions[format]
	}
	return Document{
		Format:      format,
		Filename:    filename,
		ContentType: response.Header.Get("Content-Type"),
		Body:        body,
	}, nil
}

// accepted lists the media types of the formats this service receives.
func accepted() []string {
	types := []string{}
	for _, format := range Receives {
		types = append(types, mediaTypes[format]...)
	}
	return types
}

// formatOf names the received format for a Content-Type, or reports that it
// is not one this service reads.
func formatOf(contentType string) (string, bool) {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", false
	}
	for _, format := range Receives {
		if slices.Contains(mediaTypes[format], strings.ToLower(mediaType)) {
			return format, true
		}
	}
	return "", false
}

// filenameOf reads the name the sender gave the file, from either form of
// the Content-Disposition header, and keeps only the base name.
func filenameOf(disposition string) string {
	_, params, err := mime.ParseMediaType(disposition)
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(params["filename"])
	name = strings.NewReplacer("\\", "/", "\x00", "").Replace(name)
	name = path.Base(name)
	if name == "." || name == "/" {
		return ""
	}
	return name
}

// MediaType is what a format goes out as.
func MediaType(format string) string {
	if types, ok := mediaTypes[format]; ok {
		return types[0]
	}
	return "application/octet-stream"
}

// Disposition is the Content-Disposition a served claim carries: the name in
// the RFC 8187 form, so a Korean title survives the trip whole.
func Disposition(filename string) string {
	var encoded strings.Builder
	for _, b := range []byte(filename) {
		switch {
		case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9',
			strings.IndexByte("!#$&+-.^_`|~", b) >= 0:
			encoded.WriteByte(b)
		default:
			fmt.Fprintf(&encoded, "%%%02X", b)
		}
	}
	return "attachment; filename*=UTF-8''" + encoded.String()
}

// NormalizeFormat accepts the spellings a caller might use for a format this
// service sends, and reports whether it is one.
func NormalizeFormat(raw string) (string, bool) {
	format := strings.ToLower(strings.TrimSpace(raw))
	if slices.Contains(Sends, format) {
		return format, true
	}
	return "", false
}
