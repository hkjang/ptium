package mail

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hkjang/ptium/server/internal/model"
)

// Directory turns an account id into the person behind it. The store already
// answers this for everything else, so mail keeps no user table of its own.
type Directory interface {
	GetUser(ctx context.Context, id string) (model.User, error)
}

// Delivery is one attempt to send one mail to one person: what left the
// building, when, and whether it got there. The body is not in it — a record
// that carried every notification's text would be a way to read them.
type Delivery struct {
	ID             string    `json:"id"`
	Event          string    `json:"event"`
	Recipient      string    `json:"recipient"`
	Subject        string    `json:"subject"`
	PresentationID string    `json:"presentationId,omitempty"`
	ActorID        string    `json:"actorId,omitempty"`
	Status         string    `json:"status"`
	Attempts       int       `json:"attempts"`
	ErrorMessage   string    `json:"errorMessage,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

const (
	StatusQueued = "queued"
	StatusSent   = "sent"
	StatusFailed = "failed"
)

// Ledger keeps the deliveries. The database holds them in a deployment; a
// test holds them in memory.
type Ledger interface {
	Record(ctx context.Context, delivery Delivery) error
	Complete(ctx context.Context, id, status string, attempts int, errorMessage string, at time.Time) error
	List(ctx context.Context, status string, limit int) (Page, error)
}

// Page is a listing of deliveries, newest first, with how many of each status
// there are altogether.
type Page struct {
	Items  []Delivery     `json:"items"`
	Total  int            `json:"total"`
	Status map[string]int `json:"status"`
}

type Service struct {
	ledger    Ledger
	settings  Reader
	directory Directory
	logger    *slog.Logger
	// baseURL is the deployment's own address when mail.base_url is not set.
	baseURL string
	now     func() time.Time
	send    Sender
	// pending counts deliveries still in flight, so a test can wait for them.
	pending sync.WaitGroup
}

func NewService(ledger Ledger, settings Reader, directory Directory, logger *slog.Logger, baseURL string) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{ledger: ledger, settings: settings, directory: directory, logger: logger,
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		now:     func() time.Time { return time.Now().UTC() }, send: Deliver}
}

// SetSender replaces the transport, so a test can drive the service without a
// relay.
func (s *Service) SetSender(sender Sender) { s.send = sender }

// Config is the relay as configured now, read when asked so a change on the
// settings screen applies to the next mail.
func (s *Service) Config(ctx context.Context) Config {
	config := Read(ctx, s.settings)
	if config.BaseURL == "" {
		config.BaseURL = s.baseURL
	}
	return config
}

// Notify sends one notification to some accounts, in the background. The
// request that caused it has already been answered by the time a relay is
// dialled, and a relay that is down costs the request nothing. The actor is
// never among the recipients: nobody is told about what they just did.
func (s *Service) Notify(ctx context.Context, notification Notification, actorID string, recipients []string) {
	if s == nil {
		return
	}
	config := s.Config(ctx)
	if !config.Enabled || !config.Allows(notification.Event) {
		return
	}
	addresses := s.resolve(ctx, recipients, actorID)
	if len(addresses) == 0 {
		return
	}
	body := notification.Render(config)
	for _, address := range addresses {
		delivery := s.open(ctx, notification, actorID, address)
		s.pending.Add(1)
		go func(delivery Delivery, message Message) {
			defer s.pending.Done()
			s.deliver(delivery, config, message)
		}(delivery, Message{To: address, Subject: notification.Subject, Body: body})
	}
}

// Wait blocks until every delivery started so far has been recorded. A
// process shutting down and a test both want that.
func (s *Service) Wait() { s.pending.Wait() }

// SendNow delivers one mail and says how it went, which is what the
// administrator's test button needs. It is recorded like any other.
func (s *Service) SendNow(ctx context.Context, notification Notification, actorID, recipient string) error {
	config := s.Config(ctx)
	if !config.Enabled {
		return ErrDisabled
	}
	if err := config.validate(); err != nil {
		return err
	}
	delivery := s.open(ctx, notification, actorID, recipient)
	sendContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), config.Timeout+5*time.Second)
	defer cancel()
	err := s.send(sendContext, config, Message{To: recipient, Subject: notification.Subject, Body: notification.Render(config)})
	s.close(sendContext, delivery, 1, err)
	return err
}

// Deliveries lists what was sent, newest first.
func (s *Service) Deliveries(ctx context.Context, status string, limit int) (Page, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	return s.ledger.List(ctx, strings.TrimSpace(status), limit)
}

// deliver tries twice: a relay that refuses a connection for a moment is
// common, and a lost notification costs more than a short wait.
func (s *Service) deliver(delivery Delivery, config Config, message Message) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*config.Timeout+15*time.Second)
	defer cancel()
	var err error
	attempts := 0
	for attempts < 2 {
		attempts++
		if err = s.send(ctx, config, message); err == nil {
			break
		}
		if attempts == 1 {
			select {
			case <-ctx.Done():
			case <-time.After(2 * time.Second):
			}
		}
	}
	s.close(ctx, delivery, attempts, err)
}

func (s *Service) open(ctx context.Context, notification Notification, actorID, recipient string) Delivery {
	delivery := Delivery{ID: uuid.NewString(), Event: notification.Event, Recipient: recipient,
		Subject: shorten(notification.Subject, 300), PresentationID: notification.PresentationID,
		ActorID: actorID, Status: StatusQueued, CreatedAt: s.now(), UpdatedAt: s.now()}
	if err := s.ledger.Record(context.WithoutCancel(ctx), delivery); err != nil {
		s.logger.Warn("mail delivery was not recorded", "event", delivery.Event, "error", err)
	}
	return delivery
}

func (s *Service) close(ctx context.Context, delivery Delivery, attempts int, cause error) {
	status, message := StatusSent, ""
	if cause != nil {
		status, message = StatusFailed, shorten(cause.Error(), 1000)
		// The recipient and the cause, never the credentials: the password is
		// not in the error a dial or a handshake returns, and not put there.
		s.logger.Warn("notification mail failed", "event", delivery.Event, "recipient", delivery.Recipient, "attempts", attempts, "error", cause)
	}
	if err := s.ledger.Complete(context.WithoutCancel(ctx), delivery.ID, status, attempts, message, s.now()); err != nil {
		s.logger.Warn("mail delivery outcome was not recorded", "event", delivery.Event, "error", err)
	}
}

// resolve turns account ids into distinct addresses, leaving out the actor
// and anyone who has no address or is disabled.
func (s *Service) resolve(ctx context.Context, recipients []string, actorID string) []string {
	seen := map[string]struct{}{}
	addresses := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		id := strings.TrimSpace(recipient)
		if id == "" || strings.EqualFold(id, strings.TrimSpace(actorID)) {
			continue
		}
		address := ""
		if strings.Contains(id, "@") {
			// Already an address; nothing to look up.
			address = id
		} else if s.directory != nil {
			user, err := s.directory.GetUser(ctx, id)
			if err != nil {
				s.logger.Warn("mail recipient was not resolved", "recipient", id, "error", err)
				continue
			}
			if user.Disabled {
				continue
			}
			address = strings.TrimSpace(user.Email)
		}
		if address == "" {
			continue
		}
		key := strings.ToLower(address)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		addresses = append(addresses, address)
	}
	return addresses
}
