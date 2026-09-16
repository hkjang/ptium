package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hkjang/ptium/server/internal/db"
	"github.com/hkjang/ptium/server/internal/mail"
	"github.com/hkjang/ptium/server/internal/model"
)

func TestMailSettingsAreBoundedAndThePasswordIsShippedAsASecret(t *testing.T) {
	t.Parallel()
	for key, value := range map[string]string{
		"mail.smtp_host":       `"relay.corp.example:25"`,
		"mail.smtp_port":       `0`,
		"mail.security":        `"ssl"`,
		"mail.from_address":    `"not an address"`,
		"mail.base_url":        `"ftp://slides"`,
		"mail.enabled":         `"yes"`,
		"mail.timeout_seconds": `600`,
	} {
		if err := validateSettingValue(key, json.RawMessage(value)); err == nil {
			t.Errorf("%s stored %s", key, value)
		}
	}
	for key, value := range map[string]string{
		"mail.smtp_host":    `"relay.corp.example"`,
		"mail.smtp_port":    `25`,
		"mail.security":     `"auto"`,
		"mail.from_address": `""`,
		"mail.base_url":     `""`,
		"mail.password":     `"hunter2"`,
	} {
		if err := validateSettingValue(key, json.RawMessage(value)); err != nil {
			t.Errorf("%s refused %s: %v", key, value, err)
		}
	}
	secret, shipped := db.SettingIsSecret("mail.password")
	if !shipped || !secret || !sensitiveSettingKey("mail.password") {
		t.Fatal("the SMTP password must be sealed and never read back")
	}
	if enabled, _ := db.ShippedSetting("mail.enabled"); enabled != "false" {
		t.Fatalf("shipped switched %s", enabled)
	}
}

func TestMailSwitchedOnWithoutARelayIsRefusedAtSaveTime(t *testing.T) {
	t.Parallel()
	values := map[string]json.RawMessage{"mail.enabled": json.RawMessage(`true`), "mail.smtp_host": json.RawMessage(`""`)}
	if err := mail.FromValues(values).Validate(); err == nil || !strings.Contains(err.Error(), "mail.smtp_host") {
		t.Fatalf("err = %v", err)
	}
	values["mail.smtp_host"] = json.RawMessage(`"relay.corp.example"`)
	if err := mail.FromValues(values).Validate(); err != nil {
		t.Fatalf("a relay named: %v", err)
	}
}

type offSettings struct{}

func (offSettings) Get(_ context.Context, key string, target any) error {
	if key == "mail.enabled" {
		return json.Unmarshal([]byte(`false`), target)
	}
	return json.Unmarshal([]byte(`""`), target)
}

type memoryLedger struct{}

func (memoryLedger) Record(context.Context, mail.Delivery) error { return nil }
func (memoryLedger) Complete(context.Context, string, string, int, string, time.Time) error {
	return nil
}
func (memoryLedger) List(context.Context, string, int) (mail.Page, error) { return mail.Page{}, nil }

func TestTheTestButtonSaysWhyNothingWasSent(t *testing.T) {
	t.Parallel()
	admin := model.User{ID: "admin", Email: "admin@corp.example"}
	call := func(server *Server, body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/mail/test", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		server.adminSendTestMail(recorder, request.WithContext(withUser(request.Context(), admin)))
		return recorder
	}
	// No mailer at all.
	if got := call(&Server{logger: slog.New(slog.DiscardHandler)}, `{}`); got.Code != http.StatusServiceUnavailable {
		t.Fatalf("without a mailer: %d %s", got.Code, got.Body)
	}
	// Switched off: the administrator's own address is the default recipient,
	// and the answer names the switch.
	off := &Server{logger: slog.New(slog.DiscardHandler), mail: mail.NewService(memoryLedger{}, offSettings{}, nil, nil, "")}
	if got := call(off, `{}`); got.Code != http.StatusConflict || !strings.Contains(got.Body.String(), "mail_disabled") {
		t.Fatalf("while off: %d %s", got.Code, got.Body)
	}
	if got := call(off, `{"recipient":"nobody"}`); got.Code != http.StatusUnprocessableEntity {
		t.Fatalf("a recipient with no @: %d %s", got.Code, got.Body)
	}
}
