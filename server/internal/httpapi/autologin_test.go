package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// storedFlags is a settings store holding only what a test put in it, so the
// rule can be checked without a database.
type storedFlags map[string]string

func (flags storedFlags) Get(_ context.Context, key string, target any) error {
	value, ok := flags[key]
	if !ok {
		return errors.New("no such setting")
	}
	return json.Unmarshal([]byte(value), target)
}

// Silent sign-in sends the browser to the provider before anyone has clicked
// anything, so whether it happens at all is the administrator's call and
// nobody else's. The published config says what they chose — and says no
// whenever it cannot be sure.
func TestAutoLoginIsPublishedOnlyWhenTheAdministratorTurnedItOn(t *testing.T) {
	t.Parallel()
	oidc := AuthPublicConfig{OIDCEnabled: true, ClientID: "ptium-web"}

	cases := []struct {
		name   string
		public AuthPublicConfig
		store  settingReader
		want   bool
	}{
		{"off by default", oidc, storedFlags{"auth.oidc.auto_login": "false"}, false},
		{"on when the administrator said so", oidc, storedFlags{"auth.oidc.auto_login": "true"}, true},
		{"never without OIDC, whatever the setting says", AuthPublicConfig{}, storedFlags{"auth.oidc.auto_login": "true"}, false},
		{"off when the setting cannot be read", oidc, storedFlags{}, false},
		{"off when the stored value is not a flag", oidc, storedFlags{"auth.oidc.auto_login": `"yes"`}, false},
		{"off with no settings store at all", oidc, nil, false},
		// A config assembled with the flag on, from wherever, is not believed:
		// the setting is the only thing that turns it on.
		{"not carried over from the startup config", AuthPublicConfig{OIDCEnabled: true, AutoLogin: true}, storedFlags{"auth.oidc.auto_login": "false"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := withAutoLogin(context.Background(), tc.public, tc.store)
			if got.AutoLogin != tc.want {
				t.Fatalf("autoLogin = %v, want %v", got.AutoLogin, tc.want)
			}
			// Everything else the browser reads is passed through untouched.
			got.AutoLogin, tc.public.AutoLogin = false, false
			if got != tc.public {
				t.Fatalf("the rest of the config changed: %+v", got)
			}
		})
	}
}

// The workspace acts on the flag only through the config the server publishes,
// so the flag has to be one the settings screen may store and one the schema
// names — both are checked for every flag in settingbounds_test.go. This pins
// the key itself so a rename on one side cannot silently turn the feature off.
func TestAutoLoginIsAStoredFlag(t *testing.T) {
	t.Parallel()
	if err := validateSettingValue("auth.oidc.auto_login", []byte(`true`)); err != nil {
		t.Fatalf("auth.oidc.auto_login refused true: %v", err)
	}
	if err := validateSettingValue("auth.oidc.auto_login", []byte(`"true"`)); err == nil {
		t.Fatal("auth.oidc.auto_login stored a word where the server reads a flag")
	}
}
