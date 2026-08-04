package store

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hkjang/ptium/server/internal/model"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	// ErrInvalidCredentials means the account exists but the password is wrong,
	// or no such local account exists. Callers must not distinguish the two in
	// what they tell the client.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrAccountDisabled means the credentials were correct but the account is
	// switched off.
	ErrAccountDisabled = errors.New("account is disabled")
)

// localSubjectPrefix namespaces accounts that sign in with a password, keeping
// them distinct from identity-provider subjects and dev-auth principals.
const localSubjectPrefix = "local:"

// passwordHashCost is deliberately above bcrypt's default: sign-in is rare and
// interactive, so a slower hash is cheap for us and expensive for an attacker.
const passwordHashCost = 12

// LocalSubject is the stored subject for a password account.
func LocalSubject(username string) string {
	return localSubjectPrefix + strings.ToLower(strings.TrimSpace(username))
}

// EnsureLocalAdmin seeds the bootstrap administrator. The password is written
// when the account is created and, after that, left alone: an administrator who
// changes their password in the product must not have it silently reset by the
// next restart. Pass reset to overwrite it anyway, which is the documented way
// to recover a lost password.
//
// It returns the account and whether the password was written.
func (s *Store) EnsureLocalAdmin(ctx context.Context, username, password, displayName string, reset bool) (model.User, bool, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return model.User{}, false, errors.New("bootstrap administrator username is required")
	}
	if len(password) < 12 {
		return model.User{}, false, errors.New("bootstrap administrator password must be at least 12 characters")
	}
	subject := LocalSubject(username)
	email := ""
	if strings.Contains(username, "@") {
		email = strings.ToLower(username)
	}
	if strings.TrimSpace(displayName) == "" {
		displayName = username
	}

	var existing model.User
	var storedHash []byte
	err := s.Pool.QueryRow(ctx, `SELECT `+userColumns+`,password_hash FROM users WHERE subject=$1`, subject).
		Scan(append(userScan(&existing), &storedHash)...)
	switch {
	case err == nil:
		if len(storedHash) > 0 && !reset {
			// The account already has a password. Keep it, and only make sure the
			// account is still an enabled administrator.
			if !existing.IsAdmin || existing.Disabled {
				if _, err := s.Pool.Exec(ctx, `UPDATE users SET is_admin=true,disabled=false,updated_at=now() WHERE id=$1`, existing.ID); err != nil {
					return model.User{}, false, err
				}
				existing.IsAdmin, existing.Disabled = true, false
			}
			return existing, false, nil
		}
	case errors.Is(err, pgx.ErrNoRows):
	default:
		return model.User{}, false, err
	}

	hash, hashErr := bcrypt.GenerateFromPassword([]byte(password), passwordHashCost)
	if hashErr != nil {
		return model.User{}, false, hashErr
	}
	var user model.User
	if err := s.Pool.QueryRow(ctx, `
		INSERT INTO users(subject,email,name,roles,is_admin,password_hash,password_updated_at)
		VALUES($1,$2,$3,$4,true,$5,now())
		ON CONFLICT(subject) DO UPDATE SET
			email=EXCLUDED.email,name=EXCLUDED.name,is_admin=true,disabled=false,
			roles=(SELECT ARRAY(SELECT DISTINCT unnest(users.roles || EXCLUDED.roles))),
			password_hash=EXCLUDED.password_hash,
			password_updated_at=GREATEST(now(), COALESCE(users.password_updated_at, now()) + interval '1 second'),
			updated_at=now()
		RETURNING `+userColumns,
		subject, email, displayName, []string{"ptium-admin", "user"}, hash).Scan(userScan(&user)...); err != nil {
		return model.User{}, false, fmt.Errorf("upsert bootstrap administrator: %w", err)
	}
	_, _ = s.Pool.Exec(ctx, `INSERT INTO profiles(user_id,display_name) VALUES($1,$2) ON CONFLICT DO NOTHING`, user.ID, user.Name)
	return user, true, nil
}

// AuthenticateLocalUser verifies a username and password. The hash is always
// compared, even when no account matches, so a caller cannot tell an unknown
// username from a wrong password by timing the response.
func (s *Store) AuthenticateLocalUser(ctx context.Context, username, password string) (model.User, error) {
	subject := LocalSubject(username)
	var user model.User
	var hash []byte
	err := s.Pool.QueryRow(ctx, `SELECT `+userColumns+`,password_hash FROM users
		WHERE (subject=$1 OR (email<>'' AND lower(email)=lower($2))) AND password_hash IS NOT NULL
		ORDER BY subject=$1 DESC LIMIT 1`, subject, strings.TrimSpace(username)).
		Scan(append(userScan(&user), &hash)...)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Spend the same work as a real comparison.
			_ = bcrypt.CompareHashAndPassword(decoyHash(), []byte(password))
			return model.User{}, ErrInvalidCredentials
		}
		return model.User{}, err
	}
	if bcrypt.CompareHashAndPassword(hash, []byte(password)) != nil {
		return model.User{}, ErrInvalidCredentials
	}
	if user.Disabled {
		return model.User{}, ErrAccountDisabled
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE users SET last_login=now(),updated_at=now() WHERE id=$1`, user.ID); err != nil {
		return model.User{}, err
	}
	return user, nil
}

// ChangeLocalPassword replaces an account's password after confirming the
// current one. It moves password_updated_at forward, which invalidates every
// session token issued before now.
func (s *Store) ChangeLocalPassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	var hash []byte
	if err := s.Pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE id=$1`, userID).Scan(&hash); err != nil {
		return mapNotFound(err)
	}
	if len(hash) == 0 {
		return ErrNotFound
	}
	if bcrypt.CompareHashAndPassword(hash, []byte(currentPassword)) != nil {
		return ErrInvalidCredentials
	}
	updated, err := bcrypt.GenerateFromPassword([]byte(newPassword), passwordHashCost)
	if err != nil {
		return err
	}
	// The timestamp must move even when two changes land in the same second.
	_, err = s.Pool.Exec(ctx, `UPDATE users SET password_hash=$2,
		password_updated_at=GREATEST(now(), COALESCE(password_updated_at, now()) + interval '1 second'),
		updated_at=now() WHERE id=$1`, userID, updated)
	return err
}

// HasLocalAccounts reports whether any account can sign in with a password,
// which is what decides whether the workspace offers the form at all.
func (s *Store) HasLocalAccounts(ctx context.Context) (bool, error) {
	var present bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE password_hash IS NOT NULL AND NOT disabled)`).Scan(&present)
	return present, err
}

// SessionEpoch is the password-change timestamp a session token is bound to.
// Zero means the account has no password.
func (s *Store) SessionEpoch(ctx context.Context, userID string) (int64, error) {
	var changed *time.Time
	if err := s.Pool.QueryRow(ctx, `SELECT password_updated_at FROM users WHERE id=$1`, userID).Scan(&changed); err != nil {
		return 0, mapNotFound(err)
	}
	if changed == nil {
		return 0, nil
	}
	return changed.Unix(), nil
}

// decoyHash is a real bcrypt hash of an unguessable value, generated on first
// use so an unknown username costs the same to reject as a wrong password.
var decoyHash = sync.OnceValue(func() []byte {
	filler := make([]byte, 32)
	if _, err := rand.Read(filler); err != nil {
		filler = []byte("ptium-decoy-password-material")
	}
	hash, err := bcrypt.GenerateFromPassword(filler, passwordHashCost)
	if err != nil {
		return nil
	}
	return hash
})
