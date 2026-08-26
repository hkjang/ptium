package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

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

	// DeletedDecks is what the trash is holding. Every other number here counts
	// only what has not been deleted, so a deployment can carry thousands of
	// decks nobody wanted — every draft, every trial, every import somebody
	// tried — and nothing anywhere says so. Deleting them is the operator's
	// decision, and they cannot make it without the number.
	DeletedDecks int `json:"deletedDecks"`

	// Whether generation is moving. A queue of twelve says nothing on its own:
	// twelve decks asked for in the last minute is a busy morning, and one deck
	// waiting since three hours ago is a worker that died. The age of the oldest
	// thing still waiting is the number that tells them apart.
	OldestQueuedSeconds int        `json:"oldestQueuedSeconds"`
	FailedLastDay       int        `json:"failedLastDay"`
	LastCompletedAt     *time.Time `json:"lastCompletedAt,omitempty"`
}

func (s *Store) AdminOverview(ctx context.Context) (Overview, error) {
	var result Overview
	err := s.Pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM users),
		(SELECT count(*) FROM presentations WHERE deleted_at IS NULL),
		(SELECT count(*) FROM presentations WHERE status='completed' AND deleted_at IS NULL),
		(SELECT count(*) FROM presentations WHERE status IN ('queued','generating') AND deleted_at IS NULL),
		(SELECT count(*) FROM server_errors WHERE status IN ('open','acknowledged')),
		(SELECT count(*) FROM api_keys WHERE revoked_at IS NULL AND (expires_at IS NULL OR expires_at>now()) AND (rotated_to_id IS NULL OR grace_until>now())),
		(SELECT COALESCE(EXTRACT(EPOCH FROM now()-min(updated_at))::int,0) FROM presentations
			WHERE status IN ('queued','generating') AND deleted_at IS NULL),
		(SELECT count(*) FROM presentations WHERE status='failed' AND deleted_at IS NULL AND updated_at>now()-interval '1 day'),
		(SELECT max(updated_at) FROM presentations WHERE status='completed' AND deleted_at IS NULL),
		(SELECT count(*) FROM presentations WHERE deleted_at IS NOT NULL)`).Scan(
		&result.Users, &result.Presentations, &result.CompletedDecks, &result.QueuedGenerations, &result.OpenIncidents, &result.ActiveAPIKeys,
		&result.OldestQueuedSeconds, &result.FailedLastDay, &result.LastCompletedAt, &result.DeletedDecks)
	return result, err
}

// StorageUsage is what this deployment is keeping and how much room is left.
//
// A deployment of this product is usually a box somebody owns, off the network,
// with one disk. When that disk fills, uploads and generations fail with
// whatever error the layer underneath happens to raise, and nothing in the
// administrator's screens says why. These are the numbers that say why before
// it happens.
type StorageUsage struct {
	DatabaseBytes int64        `json:"databaseBytes"`
	Tables        []TableUsage `json:"tables"`
	// AssetDir is where images are kept when they are not in the database, with
	// how much room that filesystem has left. Empty when images live in the row.
	AssetDir       string `json:"assetDir,omitempty"`
	AssetDirFree   int64  `json:"assetDirFreeBytes,omitempty"`
	AssetDirTotal  int64  `json:"assetDirTotalBytes,omitempty"`
	AssetsInVolume int64  `json:"assetsInVolume"`
	AssetsInRows   int64  `json:"assetsInRows"`
}

// TableUsage is one thing the deployment keeps: how many of it, and how much
// room it takes with its indexes.
type TableUsage struct {
	Name  string `json:"name"`
	Rows  int64  `json:"rows"`
	Bytes int64  `json:"bytes"`
}

// Storage reads what is kept and how much of it there is.
func (s *Store) Storage(ctx context.Context, assetDir string) (StorageUsage, error) {
	var usage StorageUsage
	if err := s.Pool.QueryRow(ctx, `SELECT pg_database_size(current_database())`).Scan(&usage.DatabaseBytes); err != nil {
		return usage, err
	}
	// The things that grow: decks and their slides, the pictures, the templates,
	// the revisions kept so an edit can be undone, and the trail.
	rows, err := s.Pool.Query(ctx, `SELECT relname, n_live_tup, pg_total_relation_size(relid)
		FROM pg_stat_user_tables
		WHERE relname IN ('presentations','slides','presentation_revisions','assets','templates',
			'audit_logs','server_errors','snippets','slide_comments')
		ORDER BY pg_total_relation_size(relid) DESC`)
	if err != nil {
		return usage, err
	}
	defer rows.Close()
	for rows.Next() {
		var table TableUsage
		if err := rows.Scan(&table.Name, &table.Rows, &table.Bytes); err != nil {
			return usage, err
		}
		usage.Tables = append(usage.Tables, table)
	}
	if err := rows.Err(); err != nil {
		return usage, err
	}
	// Where the pictures are. A deployment that mounted a volume for them has
	// two places to run out of room, and only one of them is the database.
	if err := s.Pool.QueryRow(ctx,
		`SELECT COALESCE(sum(size_bytes) FILTER (WHERE data IS NULL),0),
			COALESCE(sum(size_bytes) FILTER (WHERE data IS NOT NULL),0) FROM assets`).
		Scan(&usage.AssetsInVolume, &usage.AssetsInRows); err != nil {
		return usage, err
	}
	if strings.TrimSpace(assetDir) != "" {
		usage.AssetDir = assetDir
		usage.AssetDirTotal, usage.AssetDirFree = diskRoom(assetDir)
	}
	return usage, nil
}

// QueuedDeck is one generation an operator can see and act on: what it is,
// whose it is, how long it has been like that, and what went wrong if anything
// did.
type QueuedDeck struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	OwnerID      string `json:"ownerId"`
	OwnerEmail   string `json:"ownerEmail,omitempty"`
	Status       string `json:"status"`
	Stage        string `json:"stage,omitempty"`
	ErrorMessage string `json:"errorMessage,omitempty"`
	WaitingFor   int    `json:"waitingSeconds"`
	// QuietFor is how long a deck being written has said nothing, in seconds,
	// and is absent for a deck that is only waiting.
	//
	// How long a generation has been going says nothing about whether it is in
	// trouble: a self-hosted model takes minutes per call and a deployment may
	// ask for ten repair passes, so half an hour of writing can be a deck going
	// perfectly well. What separates that from a dead worker is whether anybody
	// is still saying so. The queue screen used to call anything older than
	// fifteen minutes stuck, which after this product started leaving slow
	// generations alone would have been crying wolf over healthy work.
	QuietFor  *int       `json:"quietSeconds,omitempty"`
	StartedAt *time.Time `json:"startedAt,omitempty"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// GenerationQueue is what is waiting, what is being written, and what failed
// recently.
//
// The overview learned to say that the oldest thing has been waiting twenty
// minutes; an operator reading that could do nothing with it, because a deck
// belongs to its owner and an administrator could not see one. This is the list
// behind that number.
func (s *Store) GenerationQueue(ctx context.Context, includeFailedHours int, limit int) ([]QueuedDeck, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.Pool.Query(ctx, `SELECT p.id::text, p.title, p.owner_id::text, COALESCE(u.email,''),
			p.status, p.generation_stage, COALESCE(p.error_message,''),
			EXTRACT(EPOCH FROM now()-p.updated_at)::int,
			CASE WHEN p.status='generating'
				THEN EXTRACT(EPOCH FROM now()-COALESCE(p.generation_heartbeat_at, p.generation_started_at))::int
				END,
			p.generation_started_at, p.updated_at
		FROM presentations p LEFT JOIN users u ON u.id = p.owner_id
		WHERE p.deleted_at IS NULL AND (p.status IN ('queued','generating')
			OR (p.status='failed' AND $1 > 0 AND p.updated_at > now() - ($1 || ' hours')::interval))
		ORDER BY CASE p.status WHEN 'generating' THEN 0 WHEN 'queued' THEN 1 ELSE 2 END, p.updated_at
		LIMIT $2`, includeFailedHours, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	queue := make([]QueuedDeck, 0, limit)
	for rows.Next() {
		var deck QueuedDeck
		if err := rows.Scan(&deck.ID, &deck.Title, &deck.OwnerID, &deck.OwnerEmail, &deck.Status, &deck.Stage,
			&deck.ErrorMessage, &deck.WaitingFor, &deck.QuietFor, &deck.StartedAt, &deck.UpdatedAt); err != nil {
			return nil, err
		}
		queue = append(queue, deck)
	}
	return queue, rows.Err()
}

// AuditFilter narrows the trail to the question being asked. Every field is
// optional: an operator usually arrives with one of "who changed the provider",
// "what happened to this deck" and "what did this person do".
type AuditFilter struct {
	// Action matches from the start, so "presentation" finds every
	// presentation.* entry and "presentation.delete" only the deletions.
	Action string
	// Actor is an email or a user id. An id is what the row holds and an email
	// is what an operator knows.
	Actor string
	// Target is the kind of thing acted on, and TargetID the one thing.
	Target   string
	TargetID string
	// Since keeps the trail to what happened recently. Zero means all of it.
	Since time.Time
	// Search looks through the action, the target and what was recorded with it.
	Search string
}

// ListAuditTrail reads what was written down about who did what.
//
// Thirty-five places in this server write an audit record and, until this,
// nothing read one: the trail existed and could not be opened. An operator
// asking "who turned the provider on" or "who deleted that deck" had a table
// they could only reach with psql.
func (s *Store) ListAuditTrail(ctx context.Context, filter AuditFilter, limit, offset int) ([]model.AuditEntry, int, error) {
	limit, offset = clampPage(limit, offset)
	arguments := []any{
		strings.TrimSpace(filter.Action),
		strings.TrimSpace(filter.Actor),
		strings.TrimSpace(filter.Target),
		strings.TrimSpace(filter.TargetID),
		filter.Since,
		"%" + strings.TrimSpace(filter.Search) + "%",
	}
	// An action matches itself and what it is a family of, and stops at the
	// boundary between them: "presentation" finds every presentation.* entry,
	// and "presentation.create" finds the creations without also dragging in
	// presentation.create_and_generate.
	const where = `WHERE ($1='' OR a.action = $1 OR a.action LIKE $1 || '.%')
		AND ($2='' OR u.email ILIKE '%' || $2 || '%' OR a.actor_id::text = NULLIF($2,'')::text)
		AND ($3='' OR a.target_type = $3)
		AND ($4='' OR a.target_id = $4)
		AND ($5::timestamptz IS NULL OR a.created_at >= $5)
		AND ($6='%%' OR a.action ILIKE $6 OR a.target_type ILIKE $6 OR a.target_id ILIKE $6
			OR a.metadata::text ILIKE $6 OR COALESCE(u.email,'') ILIKE $6)`
	var total int
	if err := s.Pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_logs a LEFT JOIN users u ON u.id = a.actor_id `+where,
		arguments...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT a.id, a.action, a.target_type, a.target_id, a.metadata, a.created_at,
			COALESCE(a.actor_id::text,''), COALESCE(u.email,''), COALESCE(u.name,'')
		FROM audit_logs a LEFT JOIN users u ON u.id = a.actor_id `+where+`
		ORDER BY a.created_at DESC, a.id DESC LIMIT $7 OFFSET $8`,
		append(arguments, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	entries := make([]model.AuditEntry, 0, limit)
	for rows.Next() {
		var entry model.AuditEntry
		if err := rows.Scan(&entry.ID, &entry.Action, &entry.TargetType, &entry.TargetID, &entry.Metadata,
			&entry.CreatedAt, &entry.ActorID, &entry.ActorEmail, &entry.ActorName); err != nil {
			return nil, 0, err
		}
		entries = append(entries, entry)
	}
	return entries, total, rows.Err()
}

// AuditActions are the kinds of entry the trail holds, with how many of each.
// It is what a filter offers instead of asking an operator to remember the
// names this server writes.
func (s *Store) AuditActions(ctx context.Context, since time.Time) ([]model.AuditAction, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT action, count(*)::int FROM audit_logs
		WHERE ($1::timestamptz IS NULL OR created_at >= $1)
		GROUP BY action ORDER BY count(*) DESC, action`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	actions := make([]model.AuditAction, 0)
	for rows.Next() {
		var action model.AuditAction
		if err := rows.Scan(&action.Action, &action.Count); err != nil {
			return nil, err
		}
		actions = append(actions, action)
	}
	return actions, rows.Err()
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
	pattern := likePattern(search)
	var total int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE $1='' OR email ILIKE $2`+likeEscape+` OR name ILIKE $2`+likeEscape+``, search, pattern).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT u.id::text,COALESCE(u.subject,''),u.email,u.name,u.roles,u.is_admin,u.disabled,
		COALESCE(u.last_login,u.created_at),u.created_at,u.updated_at,(u.password_hash IS NOT NULL),u.password_updated_at,
		COALESCE(p.presentation_count,0)
		FROM users u LEFT JOIN (SELECT owner_id,count(*)::int AS presentation_count FROM presentations WHERE deleted_at IS NULL GROUP BY owner_id) p ON p.owner_id=u.id
		WHERE $1='' OR u.email ILIKE $2`+likeEscape+` OR u.name ILIKE $2`+likeEscape+` ORDER BY u.created_at DESC LIMIT $3 OFFSET $4`, search, pattern, limit, offset)
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
