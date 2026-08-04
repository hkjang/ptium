package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/hkjang/ptium/server/internal/model"
)

type SettingWrite struct {
	Key         string
	Value       json.RawMessage
	Sensitive   bool
	Description string
}

type Overview struct {
	Users             int `json:"users"`
	Presentations     int `json:"presentations"`
	CompletedDecks    int `json:"completedDecks"`
	QueuedGenerations int `json:"queuedGenerations"`
	OpenIncidents     int `json:"openIncidents"`
	ActiveAPIKeys     int `json:"activeApiKeys"`
}

func (s *Store) AdminOverview(ctx context.Context) (Overview, error) {
	var result Overview
	err := s.Pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM users),
		(SELECT count(*) FROM presentations),
		(SELECT count(*) FROM presentations WHERE status='completed'),
		(SELECT count(*) FROM presentations WHERE status IN ('queued','generating')),
		(SELECT count(*) FROM server_errors WHERE status IN ('open','acknowledged')),
		(SELECT count(*) FROM api_keys WHERE revoked_at IS NULL AND (expires_at IS NULL OR expires_at>now()) AND (rotated_to_id IS NULL OR grace_until>now()))`).Scan(
		&result.Users, &result.Presentations, &result.CompletedDecks, &result.QueuedGenerations, &result.OpenIncidents, &result.ActiveAPIKeys)
	return result, err
}

func (s *Store) ListSettings(ctx context.Context) ([]model.Setting, error) {
	rows, err := s.Pool.Query(ctx, `SELECT key,value,sensitive,description,updated_by::text,updated_at FROM app_settings ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	settings := make([]model.Setting, 0)
	for rows.Next() {
		var setting model.Setting
		if err := rows.Scan(&setting.Key, &setting.Value, &setting.Sensitive, &setting.Description, &setting.UpdatedBy, &setting.UpdatedAt); err != nil {
			return nil, err
		}
		setting.Configured = len(setting.Value) > 0 && string(setting.Value) != `""` && string(setting.Value) != "null"
		settings = append(settings, setting)
	}
	return settings, rows.Err()
}

func (s *Store) GetSetting(ctx context.Context, key string) (model.Setting, error) {
	var setting model.Setting
	err := s.Pool.QueryRow(ctx, `SELECT key,value,sensitive,description,updated_by::text,updated_at FROM app_settings WHERE key=$1`, key).Scan(
		&setting.Key, &setting.Value, &setting.Sensitive, &setting.Description, &setting.UpdatedBy, &setting.UpdatedAt)
	setting.Configured = len(setting.Value) > 0 && string(setting.Value) != `""` && string(setting.Value) != "null"
	return setting, mapNotFound(err)
}

func (s *Store) PutSetting(ctx context.Context, actorID, key string, value json.RawMessage, sensitive bool, description string) (model.Setting, error) {
	if !json.Valid(value) {
		return model.Setting{}, fmt.Errorf("setting value must be valid JSON")
	}
	var setting model.Setting
	err := s.Pool.QueryRow(ctx, `INSERT INTO app_settings(key,value,sensitive,description,updated_by,updated_at)
		VALUES($1,$2,$3,$4,$5,now()) ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value,sensitive=EXCLUDED.sensitive,
		description=CASE WHEN EXCLUDED.description='' THEN app_settings.description ELSE EXCLUDED.description END,
		updated_by=EXCLUDED.updated_by,updated_at=now()
		RETURNING key,value,sensitive,description,updated_by::text,updated_at`, key, value, sensitive, description, actorID).Scan(
		&setting.Key, &setting.Value, &setting.Sensitive, &setting.Description, &setting.UpdatedBy, &setting.UpdatedAt)
	setting.Configured = true
	return setting, err
}

func (s *Store) PutSettings(ctx context.Context, actorID string, writes []SettingWrite) ([]model.Setting, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	result := make([]model.Setting, 0, len(writes))
	for _, write := range writes {
		var setting model.Setting
		err := tx.QueryRow(ctx, `INSERT INTO app_settings(key,value,sensitive,description,updated_by,updated_at)
			VALUES($1,$2,$3,$4,$5,now()) ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value,sensitive=EXCLUDED.sensitive,
			description=CASE WHEN EXCLUDED.description='' THEN app_settings.description ELSE EXCLUDED.description END,
			updated_by=EXCLUDED.updated_by,updated_at=now()
			RETURNING key,value,sensitive,description,updated_by::text,updated_at`, write.Key, write.Value, write.Sensitive, write.Description, actorID).Scan(
			&setting.Key, &setting.Value, &setting.Sensitive, &setting.Description, &setting.UpdatedBy, &setting.UpdatedAt)
		if err != nil {
			return nil, err
		}
		setting.Configured = true
		result = append(result, setting)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) ListUsers(ctx context.Context, search string, limit, offset int) ([]model.User, int, error) {
	limit, offset = clampPage(limit, offset)
	pattern := "%" + search + "%"
	var total int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE $1='' OR email ILIKE $2 OR name ILIKE $2`, search, pattern).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT u.id::text,COALESCE(u.subject,''),u.email,u.name,u.roles,u.is_admin,u.disabled,
		COALESCE(u.last_login,u.created_at),u.created_at,u.updated_at,(u.password_hash IS NOT NULL),u.password_updated_at,
		COALESCE(p.presentation_count,0)
		FROM users u LEFT JOIN (SELECT owner_id,count(*)::int AS presentation_count FROM presentations GROUP BY owner_id) p ON p.owner_id=u.id
		WHERE $1='' OR u.email ILIKE $2 OR u.name ILIKE $2 ORDER BY u.created_at DESC LIMIT $3 OFFSET $4`, search, pattern, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	users := make([]model.User, 0)
	for rows.Next() {
		var user model.User
		if err := rows.Scan(append(userScan(&user), &user.PresentationsCount)...); err != nil {
			return nil, 0, err
		}
		users = append(users, user)
	}
	return users, total, rows.Err()
}

func (s *Store) UpdateUserAdmin(ctx context.Context, id string, isAdmin, disabled bool) (model.User, error) {
	var user model.User
	err := s.Pool.QueryRow(ctx, `UPDATE users SET is_admin=$2,disabled=$3,updated_at=now() WHERE id=$1
		RETURNING `+userColumns, id, isAdmin, disabled).Scan(userScan(&user)...)
	return user, mapNotFound(err)
}

func (s *Store) CaptureIncident(ctx context.Context, incident model.Incident) error {
	if incident.ID == "" {
		incident.ID = newID()
	}
	if incident.Kind == "" {
		incident.Kind = "internal"
	}
	if incident.Severity == "" {
		incident.Severity = "error"
	}
	if len(incident.Details) == 0 {
		incident.Details = json.RawMessage(`{}`)
	}
	incident.Message = redactSecrets(incident.Message)
	incident.Details = redactJSONSecrets(incident.Details)
	if incident.Fingerprint == "" {
		stable := incident.Kind + "\x00" + normalizeError(incident.Message)
		sum := sha256.Sum256([]byte(stable))
		incident.Fingerprint = fmt.Sprintf("%x", sum[:16])
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO server_errors(id,request_id,user_id,kind,severity,message,details,fingerprint,first_occurred_at,last_occurred_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,now(),now()) ON CONFLICT(fingerprint) WHERE fingerprint <> '' AND status IN ('open','acknowledged')
		DO UPDATE SET occurrence_count=server_errors.occurrence_count+1,last_occurred_at=now(),occurred_at=now(),
		request_id=EXCLUDED.request_id,user_id=EXCLUDED.user_id,message=EXCLUDED.message,details=EXCLUDED.details,severity=EXCLUDED.severity,updated_at=now()`,
		incident.ID, incident.RequestID, incident.UserID, incident.Kind, incident.Severity, incident.Message, incident.Details, incident.Fingerprint)
	return err
}

func (s *Store) ListIncidents(ctx context.Context, status string, limit, offset int) ([]model.Incident, int, error) {
	limit, offset = clampPage(limit, offset)
	var total int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM server_errors WHERE $1='' OR status=$1`, status).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT id::text,request_id,user_id::text,kind,severity,message,details,status,notes,occurred_at,updated_at,resolved_at,resolved_by::text,
		fingerprint,occurrence_count,first_occurred_at,last_occurred_at
		FROM server_errors WHERE $1='' OR status=$1 ORDER BY occurred_at DESC LIMIT $2 OFFSET $3`, status, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	incidents := make([]model.Incident, 0)
	for rows.Next() {
		var incident model.Incident
		if err := rows.Scan(&incident.ID, &incident.RequestID, &incident.UserID, &incident.Kind, &incident.Severity, &incident.Message, &incident.Details,
			&incident.Status, &incident.Notes, &incident.OccurredAt, &incident.UpdatedAt, &incident.ResolvedAt, &incident.ResolvedBy,
			&incident.Fingerprint, &incident.OccurrenceCount, &incident.FirstOccurredAt, &incident.LastOccurredAt); err != nil {
			return nil, 0, err
		}
		incidents = append(incidents, incident)
	}
	return incidents, total, rows.Err()
}

func (s *Store) UpdateIncident(ctx context.Context, id, actorID, status string, notes *string) (model.Incident, error) {
	var incident model.Incident
	err := s.Pool.QueryRow(ctx, `UPDATE server_errors SET status=$3,notes=COALESCE($4,notes),updated_at=now(),
		resolved_at=CASE WHEN $3 IN ('resolved','ignored') THEN now() ELSE NULL END,
		resolved_by=CASE WHEN $3 IN ('resolved','ignored') THEN $2::uuid ELSE NULL END
		WHERE id=$1 RETURNING id::text,request_id,user_id::text,kind,severity,message,details,status,notes,occurred_at,updated_at,resolved_at,resolved_by::text,
		fingerprint,occurrence_count,first_occurred_at,last_occurred_at`,
		id, actorID, status, notes).Scan(&incident.ID, &incident.RequestID, &incident.UserID, &incident.Kind, &incident.Severity, &incident.Message,
		&incident.Details, &incident.Status, &incident.Notes, &incident.OccurredAt, &incident.UpdatedAt, &incident.ResolvedAt, &incident.ResolvedBy,
		&incident.Fingerprint, &incident.OccurrenceCount, &incident.FirstOccurredAt, &incident.LastOccurredAt)
	return incident, mapNotFound(err)
}

var secretPattern = regexp.MustCompile(`(?i)(bearer\s+|api[_-]?key["'=:\s]+|password["'=:\s]+|secret["'=:\s]+)([^\s,"}]+)`)
var changingTokenPattern = regexp.MustCompile(`\b[0-9a-f]{8,}\b|\b\d{4,}\b`)

func redactSecrets(value string) string {
	return secretPattern.ReplaceAllString(value, `${1}[REDACTED]`)
}

func normalizeError(value string) string {
	return strings.TrimSpace(changingTokenPattern.ReplaceAllString(redactSecrets(value), "?"))
}

func redactJSONSecrets(value json.RawMessage) json.RawMessage {
	var decoded any
	if json.Unmarshal(value, &decoded) != nil {
		return json.RawMessage(`{"redacted":true}`)
	}
	redactValue(decoded)
	result, _ := json.Marshal(decoded)
	return result
}

func redactValue(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") || strings.Contains(lower, "authorization") || strings.Contains(lower, "api_key") {
				typed[key] = "[REDACTED]"
				continue
			}
			redactValue(child)
		}
	case []any:
		for _, child := range typed {
			redactValue(child)
		}
	}
}
