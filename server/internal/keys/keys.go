package keys

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInvalidKey = errors.New("invalid API key")

type Manager struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

type Created struct {
	APIKey model.APIKey `json:"apiKey"`
	Secret string       `json:"secret"`
}

type Identity struct {
	User   model.User
	KeyID  string
	Scopes []string
}

func New(pool *pgxpool.Pool) *Manager { return &Manager{pool: pool, now: time.Now} }

func (m *Manager) Create(ctx context.Context, userID, name string, scopes []string, expiresAt *time.Time, admin bool) (Created, error) {
	if strings.TrimSpace(name) == "" {
		return Created{}, errors.New("API key name is required")
	}
	if utf8.RuneCountInString(name) > 100 {
		return Created{}, errors.New("API key name must not exceed 100 characters")
	}
	if len(scopes) == 0 {
		scopes = DefaultScopes()
	}
	if err := ValidateScopes(scopes, admin); err != nil {
		return Created{}, err
	}
	if expiresAt != nil && !expiresAt.After(m.now()) {
		return Created{}, errors.New("API key expiry must be in the future")
	}
	if expiresAt != nil && expiresAt.After(m.now().Add(2*365*24*time.Hour)) {
		return Created{}, errors.New("API key expiry must not exceed two years")
	}
	prefixBytes := make([]byte, 6)
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(prefixBytes); err != nil {
		return Created{}, err
	}
	if _, err := rand.Read(secretBytes); err != nil {
		return Created{}, err
	}
	prefix := hex.EncodeToString(prefixBytes)
	token := "ptium_" + prefix + "_" + base64.RawURLEncoding.EncodeToString(secretBytes)
	hash := sha256.Sum256([]byte(token))
	var key model.APIKey
	err := m.pool.QueryRow(ctx, `INSERT INTO api_keys(id,user_id,name,key_prefix,secret_hash,scopes,expires_at)
		VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id::text,user_id::text,name,key_prefix,scopes,expires_at,revoked_at,grace_until,last_used_at,created_at`,
		uuid.NewString(), userID, name, prefix, hash[:], scopes, expiresAt).Scan(
		&key.ID, &key.UserID, &key.Name, &key.Prefix, &key.Scopes, &key.ExpiresAt, &key.RevokedAt, &key.GraceUntil, &key.LastUsedAt, &key.CreatedAt)
	if err != nil {
		return Created{}, fmt.Errorf("create API key: %w", err)
	}
	return Created{APIKey: key, Secret: token}, nil
}

// Scope is one permission a key can carry, and what it lets a key do. The list
// is the same one ValidateScopes arbitrates on, so a scope added to the server
// appears on the screen that grants it: the screen used to carry its own list
// and had drifted, leaving templates:read — which seven routes require and
// every default key holds — impossible to grant from the product.
type Scope struct {
	ID    string `json:"id"`
	Admin bool   `json:"admin"`
	// Grants is what a key with this scope may do, in the API's own terms.
	Grants string `json:"grants"`
}

var scopeCatalogue = []Scope{
	{ID: "presentations:read", Grants: "read decks, their slides and exports"},
	{ID: "presentations:write", Grants: "create, edit, generate and delete decks"},
	{ID: "templates:read", Grants: "list templates, read one and draw its previews"},
	{ID: "templates:write", Grants: "upload, rename and delete templates"},
	{ID: "profile:read", Grants: "read the account's profile"},
	{ID: "profile:write", Grants: "change the account's profile"},
	{ID: "api_keys:manage", Grants: "list, create, rotate and revoke API keys"},
	{ID: "mcp:use", Grants: "connect an MCP client (tools also need their own scopes)"},
	{ID: "admin:settings", Admin: true, Grants: "read and change deployment settings"},
	{ID: "admin:users", Admin: true, Grants: "read the account list and change roles"},
	{ID: "admin:errors", Admin: true, Grants: "read the error centre and resolve incidents"},
}

// DefaultScopes is what a key gets when nobody chooses: enough to write decks
// from a machine, and nothing that manages the account.
func DefaultScopes() []string {
	return []string{"presentations:read", "presentations:write", "templates:read", "mcp:use"}
}

// Scopes is what this deployment may put on a key. An owner who is not an
// administrator is not offered the scopes only an administrator may hold.
func Scopes(admin bool) []Scope {
	result := make([]Scope, 0, len(scopeCatalogue))
	for _, scope := range scopeCatalogue {
		if scope.Admin && !admin {
			continue
		}
		result = append(result, scope)
	}
	return result
}

var userScopes = scopeSet(false)

var adminScopes = scopeSet(true)

// scopeSet is the catalogue as a lookup, which is what validation reads. Both
// come from the one list so neither can drift from it.
func scopeSet(admin bool) map[string]struct{} {
	set := map[string]struct{}{}
	for _, scope := range scopeCatalogue {
		if scope.Admin == admin {
			set[scope.ID] = struct{}{}
		}
	}
	return set
}

func ValidateScopes(scopes []string, admin bool) error {
	if len(scopes) > 20 {
		return errors.New("too many API key scopes")
	}
	seen := map[string]struct{}{}
	for _, scope := range scopes {
		if scope = strings.TrimSpace(scope); scope == "" {
			return errors.New("API key scopes must not be empty")
		}
		if _, duplicate := seen[scope]; duplicate {
			return fmt.Errorf("duplicate API key scope %q", scope)
		}
		seen[scope] = struct{}{}
		if _, ok := userScopes[scope]; ok {
			continue
		}
		if _, ok := adminScopes[scope]; ok {
			if !admin {
				return fmt.Errorf("scope %q requires an administrator owner", scope)
			}
			continue
		}
		return fmt.Errorf("unknown API key scope %q", scope)
	}
	return nil
}

func (m *Manager) List(ctx context.Context, userID string) ([]model.APIKey, error) {
	rows, err := m.pool.Query(ctx, `SELECT id::text,user_id::text,name,key_prefix,scopes,expires_at,revoked_at,rotated_to_id::text,grace_until,last_used_at,created_at
		FROM api_keys WHERE user_id=$1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.APIKey, 0)
	for rows.Next() {
		var key model.APIKey
		if err := rows.Scan(&key.ID, &key.UserID, &key.Name, &key.Prefix, &key.Scopes, &key.ExpiresAt, &key.RevokedAt, &key.RotatedToID, &key.GraceUntil, &key.LastUsedAt, &key.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, key)
	}
	return result, rows.Err()
}

// SetScopes changes what a key may do, without changing the key itself.
//
// A key is handed to a machine, written into its configuration and forgotten;
// making the owner issue a new one to add a permission means touching that
// machine again, so most people give every key everything instead. A revoked
// key is not changed: it is over.
func (m *Manager) SetScopes(ctx context.Context, userID, id string, scopes []string, admin bool) (model.APIKey, error) {
	if len(scopes) == 0 {
		return model.APIKey{}, errors.New("an API key needs at least one scope")
	}
	if err := ValidateScopes(scopes, admin); err != nil {
		return model.APIKey{}, err
	}
	query := `UPDATE api_keys SET scopes=$2 WHERE id=$1 AND revoked_at IS NULL`
	args := []any{id, scopes}
	if !admin {
		query += ` AND user_id=$3`
		args = append(args, userID)
	}
	query += ` RETURNING id::text,user_id::text,name,key_prefix,scopes,expires_at,revoked_at,rotated_to_id::text,grace_until,last_used_at,created_at`
	var key model.APIKey
	err := m.pool.QueryRow(ctx, query, args...).Scan(
		&key.ID, &key.UserID, &key.Name, &key.Prefix, &key.Scopes, &key.ExpiresAt, &key.RevokedAt,
		&key.RotatedToID, &key.GraceUntil, &key.LastUsedAt, &key.CreatedAt)
	if err != nil {
		return model.APIKey{}, store.ErrNotFound
	}
	return key, nil
}

func (m *Manager) Revoke(ctx context.Context, userID, id string, admin bool) error {
	query := `UPDATE api_keys SET revoked_at=now(),grace_until=NULL WHERE id=$1`
	args := []any{id}
	if !admin {
		query += ` AND user_id=$2`
		args = append(args, userID)
	}
	result, err := m.pool.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

// ScopesOf is what one key carries, for a caller that has to decide whether it
// may act on that key.
func (m *Manager) ScopesOf(ctx context.Context, userID, id string, admin bool) ([]string, error) {
	query := `SELECT scopes FROM api_keys WHERE id=$1`
	args := []any{id}
	if !admin {
		query += ` AND user_id=$2`
		args = append(args, userID)
	}
	var scopes []string
	if err := m.pool.QueryRow(ctx, query, args...).Scan(&scopes); err != nil {
		return nil, store.ErrNotFound
	}
	return scopes, nil
}

func (m *Manager) Rotate(ctx context.Context, userID, id string, admin bool, grace time.Duration) (Created, error) {
	if grace < 0 || grace > 30*24*time.Hour {
		return Created{}, errors.New("rotation grace must be between zero and 30 days")
	}
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return Created{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	query := `SELECT name,scopes,expires_at,user_id::text FROM api_keys WHERE id=$1 AND revoked_at IS NULL
		AND (expires_at IS NULL OR expires_at>now()) AND rotated_to_id IS NULL`
	args := []any{id}
	if !admin {
		query += ` AND user_id=$2`
		args = append(args, userID)
	}
	query += ` FOR UPDATE`
	var name, ownerID string
	var scopes []string
	var expiresAt *time.Time
	if err := tx.QueryRow(ctx, query, args...).Scan(&name, &scopes, &expiresAt, &ownerID); err != nil {
		return Created{}, store.ErrNotFound
	}
	prefixBytes, secretBytes := make([]byte, 6), make([]byte, 32)
	if _, err := rand.Read(prefixBytes); err != nil {
		return Created{}, err
	}
	if _, err := rand.Read(secretBytes); err != nil {
		return Created{}, err
	}
	prefix := hex.EncodeToString(prefixBytes)
	token := "ptium_" + prefix + "_" + base64.RawURLEncoding.EncodeToString(secretBytes)
	hash := sha256.Sum256([]byte(token))
	newID := uuid.NewString()
	var key model.APIKey
	if err := tx.QueryRow(ctx, `INSERT INTO api_keys(id,user_id,name,key_prefix,secret_hash,scopes,expires_at)
		VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id::text,user_id::text,name,key_prefix,scopes,expires_at,revoked_at,grace_until,last_used_at,created_at`,
		newID, ownerID, name, prefix, hash[:], scopes, expiresAt).Scan(
		&key.ID, &key.UserID, &key.Name, &key.Prefix, &key.Scopes, &key.ExpiresAt, &key.RevokedAt, &key.GraceUntil, &key.LastUsedAt, &key.CreatedAt); err != nil {
		return Created{}, err
	}
	graceUntil := m.now().UTC().Add(grace)
	if _, err := tx.Exec(ctx, `UPDATE api_keys SET rotated_to_id=$2,grace_until=$3,revoked_at=CASE WHEN $4::bigint=0 THEN now() ELSE NULL END WHERE id=$1`, id, newID, graceUntil, grace.Nanoseconds()); err != nil {
		return Created{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Created{}, err
	}
	return Created{APIKey: key, Secret: token}, nil
}

func (m *Manager) Authenticate(ctx context.Context, token string) (Identity, error) {
	prefix, ok := tokenPrefix(token)
	if !ok {
		return Identity{}, ErrInvalidKey
	}
	hash := sha256.Sum256([]byte(token))
	var identity Identity
	var storedHash []byte
	err := m.pool.QueryRow(ctx, `SELECT k.id::text,k.secret_hash,k.scopes,
		u.id::text,COALESCE(u.subject,''),u.email,u.name,u.roles,u.is_admin,u.disabled,COALESCE(u.last_login,u.created_at),u.created_at,u.updated_at
		FROM api_keys k JOIN users u ON u.id=k.user_id
		WHERE k.key_prefix=$1 AND k.revoked_at IS NULL AND (k.expires_at IS NULL OR k.expires_at>now())
		AND (k.rotated_to_id IS NULL OR k.grace_until>now())`, prefix).Scan(
		&identity.KeyID, &storedHash, &identity.Scopes, &identity.User.ID, &identity.User.Subject, &identity.User.Email,
		&identity.User.Name, &identity.User.Roles, &identity.User.IsAdmin, &identity.User.Disabled, &identity.User.LastLogin,
		&identity.User.CreatedAt, &identity.User.UpdatedAt)
	if err != nil || identity.User.Disabled || !equalHash(storedHash, hash[:]) {
		return Identity{}, ErrInvalidKey
	}
	_, _ = m.pool.Exec(ctx, `UPDATE api_keys SET last_used_at=now() WHERE id=$1`, identity.KeyID)
	return identity, nil
}

func tokenPrefix(token string) (string, bool) {
	parts := strings.SplitN(token, "_", 3)
	if len(parts) != 3 || parts[0] != "ptium" || len(parts[1]) != 12 || parts[2] == "" {
		return "", false
	}
	return parts[1], true
}

func equalHash(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var different byte
	for i := range a {
		different |= a[i] ^ b[i]
	}
	return different == 0
}
