package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// The defaults describe the common case: an internal relay on port 25 that
// takes mail from the network without credentials or TLS.
const (
	defaultPort     = 25
	defaultTimeout  = 10 * time.Second
	defaultFromName = "Ptium"

	SecurityAuto     = "auto"
	SecurityNone     = "none"
	SecurityStartTLS = "starttls"
	SecurityTLS      = "tls"
)

// The kinds of notification this product sends, and the setting that switches
// each one off. They are the things somebody actually waits on — a deck that
// takes minutes to write, a review they asked for — not everything that changes.
const (
	// EventGenerationCompleted: the deck somebody asked for is written. On a
	// self-hosted model that takes one to five minutes, which is long enough
	// to walk away from.
	EventGenerationCompleted = "generation.completed"
	// EventGenerationFailed: writing or rewriting a deck stopped with an
	// error. Without this the author finds out by coming back to look.
	EventGenerationFailed = "generation.failed"
	// EventComment: a reviewer holding a share link said something about a
	// slide. The author sent the link and is waiting for exactly this.
	EventComment = "comment.created"
	EventTest    = "test"
)

// EventSettings maps each kind to its switch. The keys are the ones every
// service in the company uses, so an operator learns them once.
var EventSettings = map[string]string{
	EventGenerationCompleted: "mail.notify_generation_completed",
	EventGenerationFailed:    "mail.notify_generation_failed",
	EventComment:             "mail.notify_comment",
}

// Reader is the little of the settings service this needs.
type Reader interface {
	Get(ctx context.Context, key string, target any) error
}

// Read maps the stored settings onto the configuration. A value that cannot
// be read is left at what the product ships, so a settings outage never turns
// mail on.
func Read(ctx context.Context, reader Reader) Config {
	config := Config{Port: defaultPort, Security: SecurityAuto, Timeout: defaultTimeout, FromName: defaultFromName, Events: map[string]bool{}}
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
	number := func(key string, target *int) {
		var value int
		if reader.Get(ctx, key, &value) == nil && value > 0 {
			*target = value
		}
	}
	flag("mail.enabled", &config.Enabled)
	word("mail.smtp_host", &config.Host)
	number("mail.smtp_port", &config.Port)
	word("mail.security", &config.Security)
	flag("mail.skip_tls_verify", &config.SkipVerify)
	word("mail.username", &config.Username)
	word("mail.password", &config.Password)
	word("mail.from_address", &config.FromAddress)
	word("mail.from_name", &config.FromName)
	word("mail.base_url", &config.BaseURL)
	seconds := 0
	number("mail.timeout_seconds", &seconds)
	if seconds > 0 {
		config.Timeout = time.Duration(seconds) * time.Second
	}
	config.Security = strings.ToLower(config.Security)
	if config.Security == "" {
		config.Security = SecurityAuto
	}
	// A relay on the implicit-TLS port needs no further configuration.
	if config.Security == SecurityAuto && config.Port == 465 {
		config.Security = SecurityTLS
	}
	if config.FromAddress == "" && config.Host != "" {
		config.FromAddress = "ptium@" + config.Host
	}
	config.BaseURL = strings.TrimRight(config.BaseURL, "/")
	// A switch appears only when it was read, so an unread one stays "send".
	for event, key := range EventSettings {
		var enabled bool
		if reader.Get(ctx, key, &enabled) == nil {
			config.Events[event] = enabled
		}
	}
	return config
}

// FromValues maps stored JSON values onto the configuration, for the write
// path checking what a save would leave behind.
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
