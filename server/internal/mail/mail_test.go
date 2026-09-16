package mail

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hkjang/ptium/server/internal/model"
)

// memoryLedger is the ledger a test reads back.
type memoryLedger struct {
	mu    sync.Mutex
	items []Delivery
}

func (l *memoryLedger) Record(_ context.Context, delivery Delivery) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.items = append(l.items, delivery)
	return nil
}

func (l *memoryLedger) Complete(_ context.Context, id, status string, attempts int, message string, at time.Time) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	for index := range l.items {
		if l.items[index].ID == id {
			l.items[index].Status, l.items[index].Attempts, l.items[index].ErrorMessage, l.items[index].UpdatedAt = status, attempts, message, at
		}
	}
	return nil
}

func (l *memoryLedger) List(_ context.Context, status string, limit int) (Page, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	page := Page{Items: []Delivery{}, Status: map[string]int{}}
	for _, item := range l.items {
		page.Status[item.Status]++
		page.Total++
		if (status == "" || item.Status == status) && len(page.Items) < limit {
			page.Items = append(page.Items, item)
		}
	}
	return page, nil
}

func (l *memoryLedger) all() []Delivery {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]Delivery(nil), l.items...)
}

type people map[string]model.User

func (p people) GetUser(_ context.Context, id string) (model.User, error) {
	user, ok := p[id]
	if !ok {
		return model.User{}, errors.New("no such user")
	}
	return user, nil
}

func values(overrides map[string]any) valueReader {
	base := map[string]any{"mail.enabled": true, "mail.smtp_host": "relay.corp.example", "mail.from_address": "ptium@corp.example"}
	for key, value := range overrides {
		base[key] = value
	}
	reader := valueReader{}
	for key, value := range base {
		raw, _ := json.Marshal(value)
		reader[key] = raw
	}
	return reader
}

var directory = people{
	"author": {ID: "author", Email: "author@corp.example"},
	"owner":  {ID: "owner", Email: "Owner@corp.example"},
	"gone":   {ID: "gone", Email: "gone@corp.example", Disabled: true},
	"blank":  {ID: "blank"},
}

// sentBy is a sender that remembers, and fails when told to.
type sentBy struct {
	mu       sync.Mutex
	messages []Message
	fail     error
}

func (s *sentBy) send(_ context.Context, _ Config, message Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		return s.fail
	}
	s.messages = append(s.messages, message)
	return nil
}

func (s *sentBy) to() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var to []string
	for _, message := range s.messages {
		to = append(to, message.To)
	}
	return to
}

func service(reader Reader, sender *sentBy) (*Service, *memoryLedger) {
	ledger := &memoryLedger{}
	s := NewService(ledger, reader, directory, slog.New(slog.DiscardHandler), "https://slides.corp.example/")
	s.SetSender(sender.send)
	return s, ledger
}

func TestTheDefaultsDescribeAnInternalRelayAndMailIsOff(t *testing.T) {
	config := Read(context.Background(), nil)
	if config.Enabled || config.Port != 25 || config.Security != SecurityAuto || config.Timeout != 10*time.Second || config.Username != "" {
		t.Fatalf("shipped as %+v", config)
	}
	if !config.Allows(EventComment) || !config.Allows("something.new") {
		t.Fatal("an unread switch must mean send")
	}
	// Port 465 is implicit TLS, and a missing sender is derived from the relay.
	config = Read(context.Background(), values(map[string]any{"mail.smtp_port": 465, "mail.from_address": ""}))
	if config.Security != SecurityTLS || config.FromAddress != "ptium@relay.corp.example" {
		t.Fatalf("465 read as %+v", config)
	}
	if err := (Config{Enabled: true, Port: 25, Security: SecurityAuto}).Validate(); err == nil {
		t.Fatal("switched on without a host must not validate")
	}
	if err := (Config{Enabled: false}).Validate(); err != nil {
		t.Fatalf("switched off is always valid: %v", err)
	}
}

func TestNothingIsSentWhileMailIsOff(t *testing.T) {
	sender := &sentBy{}
	s, ledger := service(values(map[string]any{"mail.enabled": false}), sender)
	s.Notify(context.Background(), CommentLeft("Q3", "deck", "리뷰어", 4), "", []string{"owner"})
	s.Wait()
	if len(sender.to()) != 0 || len(ledger.all()) != 0 {
		t.Fatalf("sent %v, recorded %v", sender.to(), ledger.all())
	}
	if err := s.SendNow(context.Background(), TestMessage(time.Now()), "admin", "x@corp.example"); !errors.Is(err, ErrDisabled) {
		t.Fatalf("the test button while off: %v", err)
	}
}

func TestSwitchedOnWithoutARelayIsRecordedAsFailedWithTheReason(t *testing.T) {
	sender := &sentBy{}
	s, ledger := service(values(map[string]any{"mail.smtp_host": ""}), sender)
	err := s.SendNow(context.Background(), TestMessage(time.Now()), "admin", "x@corp.example")
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "mail.smtp_host") {
		t.Fatalf("err = %v", err)
	}
	if len(ledger.all()) != 0 {
		t.Fatal("a configuration refusal is said to the caller, not queued")
	}
}

func TestADeadRelayCostsTheRequestNothingAndTheAttemptIsRecorded(t *testing.T) {
	sender := &sentBy{fail: errors.New("SMTP connection failed: connection refused")}
	s, ledger := service(values(nil), sender)
	started := time.Now()
	s.Notify(context.Background(), GenerationFailed("Q3", "deck", "모델이 답하지 않았습니다."), "", []string{"author"})
	if time.Since(started) > time.Second {
		t.Fatal("Notify waited on the relay")
	}
	s.Wait()
	items := ledger.all()
	if len(items) != 1 || items[0].Status != StatusFailed || items[0].Attempts != 2 || !strings.Contains(items[0].ErrorMessage, "refused") {
		t.Fatalf("recorded %+v", items)
	}
	if items[0].Recipient != "author@corp.example" || items[0].Event != EventGenerationFailed || items[0].PresentationID != "deck" {
		t.Fatalf("recorded %+v", items)
	}
}

func TestASentMailIsRecordedTooWithoutItsBody(t *testing.T) {
	sender := &sentBy{}
	s, ledger := service(values(nil), sender)
	s.Notify(context.Background(), GenerationCompleted("분기 보고", "deck-1", 12), "", []string{"author"})
	s.Wait()
	items := ledger.all()
	if len(items) != 1 || items[0].Status != StatusSent || items[0].Attempts != 1 || items[0].ErrorMessage != "" {
		t.Fatalf("recorded %+v", items)
	}
	if !strings.Contains(items[0].Subject, "분기 보고") {
		t.Fatalf("subject = %q", items[0].Subject)
	}
	raw, _ := json.Marshal(items[0])
	if strings.Contains(string(raw), "편집기에서") {
		t.Fatalf("the record carries the body: %s", raw)
	}
	messages := sender.messages
	if len(messages) != 1 || !strings.Contains(messages[0].Body, "https://slides.corp.example/presentations/deck-1/editor") {
		t.Fatalf("body = %q", messages[0].Body)
	}
}

func TestTheActorIsNeverToldAndAddressesAreNotRepeated(t *testing.T) {
	sender := &sentBy{}
	s, _ := service(values(nil), sender)
	s.Notify(context.Background(), CommentLeft("Q3", "deck", "", 0), "author", []string{"author", "owner", "OWNER@corp.example", "gone", "blank", "nobody", ""})
	s.Wait()
	if to := sender.to(); len(to) != 1 || to[0] != "Owner@corp.example" {
		t.Fatalf("sent to %v", to)
	}
}

func TestOneSwitchStopsOneKindOnly(t *testing.T) {
	sender := &sentBy{}
	s, ledger := service(values(map[string]any{"mail.notify_comment": false}), sender)
	s.Notify(context.Background(), CommentLeft("Q3", "deck", "리뷰어", 1), "", []string{"owner"})
	s.Notify(context.Background(), GenerationCompleted("Q3", "deck", 3), "", []string{"owner"})
	s.Wait()
	items := ledger.all()
	if len(items) != 1 || items[0].Event != EventGenerationCompleted {
		t.Fatalf("recorded %+v", items)
	}
}

func TestTheListingCountsEveryStatus(t *testing.T) {
	sender := &sentBy{}
	s, _ := service(values(nil), sender)
	s.Notify(context.Background(), GenerationCompleted("A", "a", 1), "", []string{"author"})
	s.Wait()
	sender.fail = errors.New("boom")
	s.Notify(context.Background(), GenerationCompleted("B", "b", 1), "", []string{"author"})
	s.Wait()
	page, err := s.Deliveries(context.Background(), StatusFailed, 0)
	if err != nil || page.Total != 2 || page.Status[StatusSent] != 1 || page.Status[StatusFailed] != 1 || len(page.Items) != 1 {
		t.Fatalf("page = %+v, %v", page, err)
	}
}

// fakeRelay is the least SMTP server that takes a message: no TLS, no
// authentication, which is what an internal relay on port 25 looks like.
func fakeRelay(t *testing.T) (string, int, func() string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	received := ""
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		reader := bufio.NewReader(connection)
		say := func(line string) { _, _ = connection.Write([]byte(line + "\r\n")) }
		say("220 relay.test ESMTP")
		inData := false
		var data strings.Builder
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			if inData {
				if line == "." {
					inData = false
					mu.Lock()
					received = data.String()
					mu.Unlock()
					say("250 queued")
					continue
				}
				data.WriteString(line + "\n")
				continue
			}
			switch {
			case strings.HasPrefix(strings.ToUpper(line), "EHLO"):
				say("250-relay.test")
				say("250 8BITMIME")
			case strings.HasPrefix(strings.ToUpper(line), "DATA"):
				inData = true
				say("354 go ahead")
			case strings.HasPrefix(strings.ToUpper(line), "QUIT"):
				say("221 bye")
				return
			default:
				say("250 ok")
			}
		}
	}()
	address := listener.Addr().(*net.TCPAddr)
	return address.IP.String(), address.Port, func() string {
		mu.Lock()
		defer mu.Unlock()
		return received
	}
}

func TestAMailReachesAPlainRelayOnAPortWithNoAuthenticationOrTLS(t *testing.T) {
	host, port, received := fakeRelay(t)
	config := Config{Enabled: true, Host: host, Port: port, Security: SecurityAuto, FromAddress: "ptium@corp.example", FromName: "Ptium", Timeout: 3 * time.Second}
	err := Deliver(context.Background(), config, Message{To: "author@corp.example", Subject: "덱이 완성되었습니다", Body: "본문\n둘째 줄"})
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for received() == "" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	got := received()
	for _, want := range []string{"From: Ptium <ptium@corp.example>", "To: author@corp.example", "Subject: =?utf-8?q?", "Content-Type: text/plain; charset=utf-8", "본문\n둘째 줄"} {
		if !strings.Contains(got, want) {
			t.Errorf("the relay got no %q in:\n%s", want, got)
		}
	}
}

func TestARelayThatIsNotThereFailsWithinTheTimeout(t *testing.T) {
	listener, _ := net.Listen("tcp", "127.0.0.1:0")
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	config := Config{Enabled: true, Host: "127.0.0.1", Port: port, Security: SecurityNone, FromAddress: "ptium@corp.example", Timeout: time.Second}
	started := time.Now()
	err := Deliver(context.Background(), config, Message{To: "x@corp.example", Subject: "s", Body: "b"})
	if err == nil || !strings.Contains(err.Error(), "SMTP connection failed") || time.Since(started) > 5*time.Second {
		t.Fatalf("err = %v after %s", err, time.Since(started))
	}
}

func TestStartTLSDemandedOfARelayWithoutItIsRefusedBeforeAnythingIsSent(t *testing.T) {
	host, port, received := fakeRelay(t)
	config := Config{Enabled: true, Host: host, Port: port, Security: SecurityStartTLS, FromAddress: "ptium@corp.example", Timeout: 3 * time.Second}
	err := Deliver(context.Background(), config, Message{To: "x@corp.example", Subject: "s", Body: "b"})
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "STARTTLS") || received() != "" {
		t.Fatalf("err = %v, received %q", err, received())
	}
}
