package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")
var ErrGenerationLimit = errors.New("presentation exceeds the configured generation slide limit")
var ErrConflict = errors.New("resource version conflict")

// ErrValidation marks what the caller sent being wrong, as opposed to the server
// failing. Everything wrapping it is answered as a 422 rather than filed as an
// incident.
var ErrValidation = errors.New("invalid input")

type Store struct {
	Pool *pgxpool.Pool
	// Blobs holds uploaded image bytes when a deployment gives Ptium a volume
	// for them. Nil keeps them in the assets row, which is the default and needs
	// nothing mounted.
	Blobs BlobStore
}

func New(pool *pgxpool.Pool) *Store { return &Store{Pool: pool} }

// WithBlobs points the store at a place to keep uploaded image bytes.
func (s *Store) WithBlobs(blobs BlobStore) *Store {
	s.Blobs = blobs
	return s
}

func mapNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	// An id that is not a UUID cannot name a row. The router checks the ids in a
	// path, but ids also arrive in request bodies, and "invalid input syntax for
	// type uuid" reaching a caller as a five hundred blames the server for what
	// the caller sent.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "22P02" {
		return ErrNotFound
	}
	return err
}

func (s *Store) Ping(ctx context.Context) error { return s.Pool.Ping(ctx) }

// userColumns and userScan keep every user query reading the same shape.
const userColumns = `id::text,COALESCE(subject,''),email,name,roles,is_admin,disabled,
	COALESCE(last_login,created_at),created_at,updated_at,(password_hash IS NOT NULL),password_updated_at`

func userScan(user *model.User) []any {
	return []any{&user.ID, &user.Subject, &user.Email, &user.Name, &user.Roles, &user.IsAdmin,
		&user.Disabled, &user.LastLogin, &user.CreatedAt, &user.UpdatedAt, &user.HasPassword, &user.PasswordUpdatedAt}
}

// UpsertUser records the person behind a request, creating them on first sight.
//
// Every request that carries no user id in its claims comes through here, so a
// browser opening the workspace runs it several times at once — and the first
// time a person signs in, those requests race to create the same row. The
// statement below names the subject index as the one it arbitrates on, so the
// insert that loses collides with the unique index on the email instead, which
// it is not arbitrating on, and Postgres raises rather than updating. One in a
// handful of first sign-ins answered five hundred.
//
// The loser only has to look again: by then the winner's row is there, and the
// same statement takes its update path.
func (s *Store) UpsertUser(ctx context.Context, subject, email, name string, roles []string, admin bool) (model.User, error) {
	user, err := s.upsertUserOnce(ctx, subject, email, name, roles, admin)
	if isUniqueViolation(err) {
		user, err = s.upsertUserOnce(ctx, subject, email, name, roles, admin)
		if isUniqueViolation(err) {
			// Not a race then: this address already belongs to another identity.
			// One account per address is this product's rule, and a person who
			// hits it needs to be told, not handed a five hundred.
			return model.User{}, fmt.Errorf("%w: this email address already belongs to another sign-in identity", ErrConflict)
		}
	}
	if err != nil {
		return model.User{}, fmt.Errorf("upsert user: %w", err)
	}
	_, _ = s.Pool.Exec(ctx, `INSERT INTO profiles(user_id,display_name) VALUES($1,$2) ON CONFLICT DO NOTHING`, user.ID, user.Name)
	return user, nil
}

func (s *Store) upsertUserOnce(ctx context.Context, subject, email, name string, roles []string, admin bool) (model.User, error) {
	var user model.User
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO users(subject,email,name,roles,is_admin,last_login)
		VALUES($1,$2,$3,$4,$5,now())
		ON CONFLICT(subject) DO UPDATE SET
			email=EXCLUDED.email,name=EXCLUDED.name,roles=EXCLUDED.roles,
			is_admin=(users.is_admin OR EXCLUDED.is_admin),last_login=now(),updated_at=now()
		RETURNING `+userColumns,
		subject, email, name, roles, admin).Scan(userScan(&user)...)
	return user, err
}

// isUniqueViolation reports the one Postgres error this has to tell apart.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func (s *Store) GetUser(ctx context.Context, id string) (model.User, error) {
	var user model.User
	err := s.Pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id=$1`, id).Scan(userScan(&user)...)
	return user, mapNotFound(err)
}

func (s *Store) GetProfile(ctx context.Context, userID string) (model.Profile, error) {
	var profile model.Profile
	err := s.Pool.QueryRow(ctx, `SELECT user_id::text,display_name,company,job_title,bio,preferences,updated_at FROM profiles WHERE user_id=$1`, userID).Scan(
		&profile.UserID, &profile.DisplayName, &profile.Company, &profile.JobTitle, &profile.Bio, &profile.Preferences, &profile.UpdatedAt)
	return profile, mapNotFound(err)
}

func (s *Store) UpdateProfile(ctx context.Context, userID string, profile model.Profile) (model.Profile, error) {
	if len(profile.Preferences) == 0 {
		profile.Preferences = json.RawMessage(`{}`)
	}
	err := s.Pool.QueryRow(ctx, `INSERT INTO profiles(user_id,display_name,company,job_title,bio,preferences,updated_at)
		VALUES($1,$2,$3,$4,$5,$6,now()) ON CONFLICT(user_id) DO UPDATE SET
		display_name=EXCLUDED.display_name,company=EXCLUDED.company,job_title=EXCLUDED.job_title,
		bio=EXCLUDED.bio,preferences=EXCLUDED.preferences,updated_at=now()
		RETURNING user_id::text,display_name,company,job_title,bio,preferences,updated_at`,
		userID, profile.DisplayName, profile.Company, profile.JobTitle, profile.Bio, profile.Preferences).Scan(
		&profile.UserID, &profile.DisplayName, &profile.Company, &profile.JobTitle, &profile.Bio, &profile.Preferences, &profile.UpdatedAt)
	return profile, err
}

func (s *Store) Audit(ctx context.Context, actorID *string, action, targetType, targetID string, metadata any) {
	data, _ := json.Marshal(metadata)
	_, _ = s.Pool.Exec(ctx, `INSERT INTO audit_logs(actor_id,action,target_type,target_id,metadata) VALUES($1,$2,$3,$4,$5)`, actorID, action, targetType, targetID, data)
}

func newID() string { return uuid.NewString() }

func clampPage(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
