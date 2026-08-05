package settings

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/store"
)

type Service struct {
	store *store.Store
	aead  cipher.AEAD
}

// ErrUnreadable reports a sensitive setting whose stored value cannot be
// decrypted, almost always because the encryption key changed. Callers treat it
// as "not configured" rather than as a server fault.
var ErrUnreadable = errors.New("the stored value could not be decrypted; enter it again in the admin settings")

type sealedValue struct {
	Version    int    `json:"version"`
	Ciphertext string `json:"ciphertext"`
}

type Update struct {
	Key         string
	Value       json.RawMessage
	Sensitive   bool
	Description string
}

func New(st *store.Store, keyMaterial string) (*Service, error) {
	digest := sha256.Sum256([]byte("ptium/settings/v1\x00" + keyMaterial))
	block, err := aes.NewCipher(digest[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Service{store: st, aead: aead}, nil
}

func (s *Service) ListForAdmin(ctx context.Context) ([]model.Setting, error) {
	result, err := s.store.ListSettings(ctx)
	if err != nil {
		return nil, err
	}
	for i := range result {
		if result[i].Sensitive {
			plain, openErr := s.open(result[i].Value)
			if openErr != nil {
				// One key the server cannot decrypt must not take the settings
				// page down with it: that is precisely the moment an operator
				// needs the page in order to type the key in again.
				result[i].Unreadable = true
				result[i].Configured = false
				result[i].Value = nil
				continue
			}
			result[i].Configured = configuredValue(plain)
			result[i].Value = nil
		}
	}
	return result, nil
}

func (s *Service) Put(ctx context.Context, actorID, key string, value json.RawMessage, sensitive bool, description string) (model.Setting, error) {
	if len(value) == 0 || !json.Valid(value) {
		return model.Setting{}, errors.New("value must be valid JSON")
	}
	stored := value
	if sensitive {
		sealed, err := s.seal(value)
		if err != nil {
			return model.Setting{}, err
		}
		stored = sealed
	}
	setting, err := s.store.PutSetting(ctx, actorID, key, stored, sensitive, description)
	if err == nil && setting.Sensitive {
		setting.Configured = configuredValue(value)
		setting.Value = nil
	}
	return setting, err
}

func (s *Service) PutBatch(ctx context.Context, actorID string, updates []Update) ([]model.Setting, error) {
	writes := make([]store.SettingWrite, 0, len(updates))
	configured := make(map[string]bool, len(updates))
	for _, update := range updates {
		if len(update.Value) == 0 || !json.Valid(update.Value) {
			return nil, fmt.Errorf("setting %q value must be valid JSON", update.Key)
		}
		stored := update.Value
		if update.Sensitive {
			sealed, err := s.seal(update.Value)
			if err != nil {
				return nil, err
			}
			stored = sealed
			configured[update.Key] = configuredValue(update.Value)
		}
		writes = append(writes, store.SettingWrite{Key: update.Key, Value: stored, Sensitive: update.Sensitive, Description: update.Description})
	}
	result, err := s.store.PutSettings(ctx, actorID, writes)
	if err != nil {
		return nil, err
	}
	for index := range result {
		if result[index].Sensitive {
			result[index].Configured = configured[result[index].Key]
			result[index].Value = nil
		}
	}
	return result, nil
}

func (s *Service) Get(ctx context.Context, key string, target any) error {
	setting, err := s.store.GetSetting(ctx, key)
	if err != nil {
		return err
	}
	value := setting.Value
	if setting.Sensitive {
		value, err = s.open(value)
		if err != nil {
			return fmt.Errorf("setting %q: %w", key, ErrUnreadable)
		}
	}
	if err := json.Unmarshal(value, target); err != nil {
		return fmt.Errorf("decode setting %q: %w", key, err)
	}
	return nil
}

func (s *Service) Public(ctx context.Context) (map[string]any, error) {
	all, err := s.store.ListSettings(ctx)
	if err != nil {
		return nil, err
	}
	result := map[string]any{}
	for _, setting := range all {
		if setting.Sensitive || (!hasPrefix(setting.Key, "branding.") && !hasPrefix(setting.Key, "generation.")) {
			continue
		}
		var value any
		if json.Unmarshal(setting.Value, &value) == nil {
			result[setting.Key] = value
		}
	}
	return result, nil
}

func (s *Service) seal(plain []byte) (json.RawMessage, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	sealed := s.aead.Seal(nonce, nonce, plain, []byte("ptium-setting"))
	return json.Marshal(sealedValue{Version: 1, Ciphertext: base64.RawStdEncoding.EncodeToString(sealed)})
}

func (s *Service) open(value json.RawMessage) (json.RawMessage, error) {
	var sealed sealedValue
	if err := json.Unmarshal(value, &sealed); err != nil || sealed.Version != 1 {
		// Compatibility for an initial plaintext value such as the empty seed.
		if json.Valid(value) {
			return value, nil
		}
		return nil, errors.New("unsupported encrypted value")
	}
	encoded, err := base64.RawStdEncoding.DecodeString(sealed.Ciphertext)
	if err != nil || len(encoded) < s.aead.NonceSize() {
		return nil, errors.New("invalid encrypted value")
	}
	nonce := encoded[:s.aead.NonceSize()]
	plain, err := s.aead.Open(nil, nonce, encoded[s.aead.NonceSize():], []byte("ptium-setting"))
	if err != nil {
		return nil, errors.New("encrypted value authentication failed")
	}
	return plain, nil
}

func hasPrefix(value, prefix string) bool {
	return len(value) >= len(prefix) && value[:len(prefix)] == prefix
}

func configuredValue(value json.RawMessage) bool {
	trimmed := string(value)
	return len(value) > 0 && trimmed != `""` && trimmed != "null"
}
