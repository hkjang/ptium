package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")
var ErrGenerationLimit = errors.New("presentation exceeds the configured generation slide limit")

type Store struct {
	Pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store { return &Store{Pool: pool} }

func mapNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
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

func (s *Store) UpsertUser(ctx context.Context, subject, email, name string, roles []string, admin bool) (model.User, error) {
	var user model.User
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO users(subject,email,name,roles,is_admin,last_login)
		VALUES($1,$2,$3,$4,$5,now())
		ON CONFLICT(subject) DO UPDATE SET
			email=EXCLUDED.email,name=EXCLUDED.name,roles=EXCLUDED.roles,
			is_admin=(users.is_admin OR EXCLUDED.is_admin),last_login=now(),updated_at=now()
		RETURNING `+userColumns,
		subject, email, name, roles, admin).Scan(userScan(&user)...)
	if err != nil {
		return model.User{}, fmt.Errorf("upsert user: %w", err)
	}
	_, _ = s.Pool.Exec(ctx, `INSERT INTO profiles(user_id,display_name) VALUES($1,$2) ON CONFLICT DO NOTHING`, user.ID, user.Name)
	return user, nil
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
