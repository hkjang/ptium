package httpapi

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The API answers in English on purpose: the same sentence is read from a
// request log and by whoever is writing against it. The workspace turns it into
// the reader's words in web/src/api/errors.ts, keyed on the message.
//
// Nothing kept the two in step. Twenty-four messages had no rule written for
// them and arrived in English on a screen that is Korean throughout — among
// them the one an offline site meets on every attempt to write a deck, "This
// deployment has no AI provider configured".
//
// Skipped when the web source is not beside the server.
func TestEveryRefusalAPersonMeetsIsWrittenInTheirWords(t *testing.T) {
	rules, err := os.ReadFile(filepath.Join("..", "..", "..", "web", "src", "api", "errors.ts"))
	if err != nil {
		t.Skip("the web source is not beside the server here")
	}
	written := string(rules)

	// The two answers that stay in English: the OIDC token endpoint is read by
	// whoever is wiring an identity provider, never by a person on a screen.
	forIntegrators := map[string]bool{
		"token_exchange_unavailable": true,
		"unsupported_grant_type":     true,
	}

	handlers, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	message := regexp.MustCompile(`writeError\([^;]*?"([a-z_]+)",\s*"([^"]{5,})"`)
	missing := 0
	for _, file := range handlers {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, found := range message.FindAllStringSubmatch(string(source), -1) {
			code, said := found[1], found[2]
			if forIntegrators[code] || hasHangul(said) {
				continue
			}
			// Written against the message, or against the code as a fallback.
			if strings.Contains(written, said) || regexp.MustCompile(`(?m)^\s*`+regexp.QuoteMeta(code)+`:\s*'`).MatchString(written) {
				continue
			}
			missing++
			t.Errorf("%s: [%s] %q reaches a screen with no rule written for it", file, code, said)
		}
	}
	if missing > 0 {
		t.Logf("%d messages arrive in English on a Korean screen; write them in web/src/api/errors.ts", missing)
	}
}

func hasHangul(text string) bool {
	for _, letter := range text {
		if letter >= 0xAC00 && letter <= 0xD7A3 {
			return true
		}
	}
	return false
}
