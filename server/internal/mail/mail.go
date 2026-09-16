// Package mail sends event notifications through a company SMTP relay.
//
// An internal relay commonly takes mail on port 25 with no credentials and no
// TLS, so that is what the defaults describe; authentication and encryption
// are used when the relay offers them. Nothing here holds a request up: a
// notification is sent in the background, and every attempt — sent or not —
// is recorded so an administrator can answer "it never came".
package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

var (
	ErrDisabled = errors.New("mail is disabled")
	ErrInvalid  = errors.New("invalid mail configuration")
)

// Config is the relay as the administrator described it.
type Config struct {
	Enabled     bool
	Host        string
	Port        int
	Security    string
	SkipVerify  bool
	Username    string
	Password    string
	FromAddress string
	FromName    string
	BaseURL     string
	Timeout     time.Duration
	// Events is the administrator's switch for each kind of notification. A
	// kind not in it is sent: adding a notification never needs a setting first.
	Events map[string]bool
}

// Allows reports whether one kind of notification is to be sent.
func (c Config) Allows(event string) bool {
	if enabled, known := c.Events[event]; known {
		return enabled
	}
	return true
}

// Address is the From header value.
func (c Config) Address() string {
	from := strings.TrimSpace(c.FromAddress)
	if name := strings.TrimSpace(c.FromName); name != "" {
		return fmt.Sprintf("%s <%s>", name, from)
	}
	return from
}

func (c Config) endpoint() string { return net.JoinHostPort(c.Host, fmt.Sprint(c.Port)) }

// Validate says why this configuration cannot send, or nil. It is what the
// settings screen refuses a save on when mail is switched on with nothing to
// send through, and what a delivery is recorded as failing on otherwise.
func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	return c.validate()
}

func (c Config) validate() error {
	if strings.TrimSpace(c.Host) == "" {
		return fmt.Errorf("%w: mail.smtp_host is required", ErrInvalid)
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("%w: mail.smtp_port must be between 1 and 65535", ErrInvalid)
	}
	if !strings.Contains(c.FromAddress, "@") {
		return fmt.Errorf("%w: mail.from_address must be an email address", ErrInvalid)
	}
	switch c.Security {
	case SecurityAuto, SecurityNone, SecurityStartTLS, SecurityTLS:
	default:
		return fmt.Errorf("%w: mail.security must be auto, none, starttls or tls", ErrInvalid)
	}
	return nil
}

// Message is one mail to one person.
type Message struct {
	To      string
	Subject string
	Body    string
}

// Sender delivers one message; the service takes a real relay by default and a
// test hands in one that remembers what it was given.
type Sender func(context.Context, Config, Message) error

// Deliver opens a connection to the relay and sends one message.
func Deliver(ctx context.Context, config Config, message Message) error {
	if err := config.validate(); err != nil {
		return err
	}
	if strings.TrimSpace(message.To) == "" {
		return fmt.Errorf("%w: recipient is required", ErrInvalid)
	}
	client, err := dial(ctx, config)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	if err := startSession(client, config); err != nil {
		return err
	}
	if err := client.Mail(strings.TrimSpace(config.FromAddress)); err != nil {
		return fmt.Errorf("MAIL FROM refused: %w", err)
	}
	if err := client.Rcpt(strings.TrimSpace(message.To)); err != nil {
		return fmt.Errorf("RCPT TO refused: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("DATA refused: %w", err)
	}
	if _, err := writer.Write([]byte(compose(config, message))); err != nil {
		return fmt.Errorf("body not sent: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("body not accepted: %w", err)
	}
	return client.Quit()
}

func dial(ctx context.Context, config Config) (*smtp.Client, error) {
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	dialer := &net.Dialer{Timeout: timeout}
	var connection net.Conn
	var err error
	if config.Security == SecurityTLS {
		connection, err = tls.DialWithDialer(dialer, "tcp", config.endpoint(), config.tlsConfig())
	} else {
		connection, err = dialer.DialContext(ctx, "tcp", config.endpoint())
	}
	if err != nil {
		return nil, fmt.Errorf("SMTP connection failed: %w", err)
	}
	// A relay that answers the connection and then says nothing would hold
	// the sender for as long as the operating system allows; every step of the
	// dialogue is bounded instead.
	_ = connection.SetDeadline(time.Now().Add(timeout))
	client, err := smtp.NewClient(connection, config.Host)
	if err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("SMTP session failed: %w", err)
	}
	return client, nil
}

// startSession upgrades and authenticates only as far as the relay offers, so
// an internal relay that takes anything and a provider that wants both work
// off the same settings.
func startSession(client *smtp.Client, config Config) error {
	if err := client.Hello(helloName(config)); err != nil {
		return fmt.Errorf("EHLO refused: %w", err)
	}
	if config.Security == SecurityStartTLS || config.Security == SecurityAuto {
		if supported, _ := client.Extension("STARTTLS"); supported {
			if err := client.StartTLS(config.tlsConfig()); err != nil {
				return fmt.Errorf("STARTTLS failed: %w", err)
			}
		} else if config.Security == SecurityStartTLS {
			return fmt.Errorf("%w: the relay does not offer STARTTLS", ErrInvalid)
		}
	}
	if strings.TrimSpace(config.Username) == "" {
		return nil
	}
	supported, mechanisms := client.Extension("AUTH")
	if !supported {
		return fmt.Errorf("%w: the relay does not offer authentication; leave the username empty", ErrInvalid)
	}
	offered := strings.ToUpper(mechanisms)
	switch {
	case strings.Contains(offered, "PLAIN"):
		return client.Auth(smtp.PlainAuth("", config.Username, config.Password, config.Host))
	case strings.Contains(offered, "LOGIN"):
		return client.Auth(loginAuth{username: config.Username, password: config.Password, host: config.Host})
	default:
		return client.Auth(smtp.CRAMMD5Auth(config.Username, config.Password))
	}
}

func (c Config) tlsConfig() *tls.Config {
	return &tls.Config{ServerName: c.Host, MinVersion: tls.VersionTLS12, InsecureSkipVerify: c.SkipVerify} //nolint:gosec // opt-in, for an internal relay with a private certificate
}

// helloName is the sender's domain, which a relay that checks the greeting
// takes better than a container hostname.
func helloName(config Config) string {
	if at := strings.LastIndex(config.FromAddress, "@"); at >= 0 && at+1 < len(config.FromAddress) {
		return config.FromAddress[at+1:]
	}
	return "localhost"
}

// loginAuth is the LOGIN mechanism, which several corporate relays offer
// instead of PLAIN; the standard library ships only PLAIN and CRAM-MD5.
type loginAuth struct{ username, password, host string }

func (a loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if !server.TLS && server.Name != a.host {
		return "", nil, errors.New("LOGIN authentication is only sent to the relay that was asked for")
	}
	return "LOGIN", nil, nil
}

func (a loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	switch strings.ToLower(strings.TrimRight(string(fromServer), ": ")) {
	case "username":
		return []byte(a.username), nil
	case "password":
		return []byte(a.password), nil
	}
	return nil, fmt.Errorf("unexpected LOGIN prompt %q", fromServer)
}
