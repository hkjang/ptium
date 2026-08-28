package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
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

	// How many of those open faults this build has actually seen. An error
	// centre with eight open groups says nothing about whether this deployment
	// is broken: most of them can belong to builds the site upgraded past
	// months ago. The two numbers together are the ones worth acting on.
	// Zero when the process was built without a version stamp, because then
	// nothing can be attributed to it.
	OpenIncidentsThisBuild int `json:"openIncidentsThisBuild"`
	// And how many are recorded against some other build. The two do not have
	// to add up to the open count: a group recorded before the product kept the
	// build belongs to neither, and calling those "an earlier version" would be
	// saying something nobody knows.
	OpenIncidentsOtherBuild int `json:"openIncidentsOtherBuild"`

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
	OldestQueuedSeconds int `json:"oldestQueuedSeconds"`
	// QuietestGenerationSeconds is the longest any deck being written has gone
	// without its worker saying it is alive.
	//
	// The wait above used to count decks being written as well, so a deck thirty
	// minutes into a perfectly good generation read as "가장 오래 기다린 덱
	// 30분 — 작업자를 확인하세요" with a stall warning beside it. How long a
	// generation has been running is not a fault; silence is.
	QuietestGenerationSeconds int        `json:"quietestGenerationSeconds"`
	FailedLastDay             int        `json:"failedLastDay"`
	LastCompletedAt           *time.Time `json:"lastCompletedAt,omitempty"`
}

func (s *Store) AdminOverview(ctx context.Context) (Overview, error) {
	var result Overview
	err := s.Pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM users),
		(SELECT count(*) FROM presentations WHERE deleted_at IS NULL),
		(SELECT count(*) FROM presentations WHERE status='completed' AND deleted_at IS NULL),
		(SELECT count(*) FROM presentations WHERE status IN ('queued','generating') AND deleted_at IS NULL),
		(SELECT count(*) FROM server_errors WHERE status IN ('open','acknowledged')),
		(SELECT count(*) FROM server_errors WHERE status IN ('open','acknowledged') AND last_seen_version<>'' AND last_seen_version=$1),
		(SELECT count(*) FROM server_errors WHERE status IN ('open','acknowledged') AND last_seen_version<>'' AND last_seen_version<>$1),
		(SELECT count(*) FROM api_keys WHERE revoked_at IS NULL AND (expires_at IS NULL OR expires_at>now()) AND (rotated_to_id IS NULL OR grace_until>now())),
		(SELECT COALESCE(EXTRACT(EPOCH FROM now()-min(updated_at))::int,0) FROM presentations
			WHERE status='queued' AND deleted_at IS NULL),
		(SELECT COALESCE(MAX(EXTRACT(EPOCH FROM now()-COALESCE(generation_heartbeat_at,generation_started_at)))::int,0)
			FROM presentations WHERE status='generating' AND deleted_at IS NULL),
		(SELECT count(*) FROM presentations WHERE status='failed' AND deleted_at IS NULL AND updated_at>now()-interval '1 day'),
		(SELECT max(updated_at) FROM presentations WHERE status='completed' AND deleted_at IS NULL),
		(SELECT count(*) FROM presentations WHERE deleted_at IS NOT NULL)`, s.Version).Scan(
		&result.Users, &result.Presentations, &result.CompletedDecks, &result.QueuedGenerations, &result.OpenIncidents, &result.OpenIncidentsThisBuild, &result.OpenIncidentsOtherBuild, &result.ActiveAPIKeys,
		&result.OldestQueuedSeconds, &result.QuietestGenerationSeconds, &result.FailedLastDay,
		&result.LastCompletedAt, &result.DeletedDecks)
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
// QueueTotals is how much there is, whatever the list was able to carry.
//
// The screen used to count the rows it had been handed, and it is handed a
// hundred: a site with three hundred decks waiting read "100 대기 · 작성 중"
// on the very screen an operator opens to see how far behind it is, while the
// overview beside it counted them all and said three hundred.
type QueueTotals struct {
	// Waiting is everything queued or being written, uncapped.
	Waiting int `json:"waiting"`
	// Failed is everything that failed inside the window asked for, uncapped.
	Failed int `json:"failed"`
}

// GenerationQueueTotals counts what the queue holds, over the same conditions
// the list is drawn from.
func (s *Store) GenerationQueueTotals(ctx context.Context, includeFailedHours int) (QueueTotals, error) {
	var totals QueueTotals
	err := s.Pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM presentations WHERE deleted_at IS NULL AND status IN ('queued','generating')),
		(SELECT count(*) FROM presentations WHERE deleted_at IS NULL AND status='failed'
			AND $1 > 0 AND updated_at > now() - ($1 || ' hours')::interval)`,
		includeFailedHours).Scan(&totals.Waiting, &totals.Failed)
	return totals, err
}

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

// AuditEntry reads one thing that was written down, by its id. Acting on a
// trail entry — putting a setting back to what it was before this change —
// needs the entry itself, not a page of the trail it sits on.
func (s *Store) AuditEntry(ctx context.Context, id int64) (model.AuditEntry, error) {
	var entry model.AuditEntry
	err := s.Pool.QueryRow(ctx, `SELECT a.id, COALESCE(a.actor_id::text,''), COALESCE(u.email,''), COALESCE(u.name,''),
			a.action, a.target_type, a.target_id, a.metadata, a.created_at
		FROM audit_logs a LEFT JOIN users u ON u.id = a.actor_id WHERE a.id=$1`, id).Scan(
		&entry.ID, &entry.ActorID, &entry.ActorEmail, &entry.ActorName,
		&entry.Action, &entry.TargetType, &entry.TargetID, &entry.Metadata, &entry.CreatedAt)
	if err != nil {
		return model.AuditEntry{}, mapNotFound(err)
	}
	return entry, nil
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
	// The build that saw it. A repeat of the same fault moves the last-seen
	// version forward and never touches the first, so a fault that survived an
	// upgrade reads differently from one that stopped at a release. A group
	// recorded before the product kept the build keeps a blank first version
	// when it happens again: blank there means nobody knows which build saw it
	// first, and today's is not the answer.
	//
	// A process that cannot say which build it is writes no build at all, in
	// either column. Letting it write an empty last-seen version would erase
	// what an earlier, stamped process knew — turning an answer into a blank.
	_, err := s.Pool.Exec(ctx, `INSERT INTO server_errors(id,request_id,user_id,kind,severity,message,details,fingerprint,first_occurred_at,last_occurred_at,first_seen_version,last_seen_version)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,now(),now(),$9,$9) ON CONFLICT(fingerprint) WHERE fingerprint <> '' AND status IN ('open','acknowledged')
		DO UPDATE SET occurrence_count=server_errors.occurrence_count+1,last_occurred_at=now(),occurred_at=now(),
		request_id=EXCLUDED.request_id,user_id=EXCLUDED.user_id,message=EXCLUDED.message,details=EXCLUDED.details,severity=EXCLUDED.severity,updated_at=now(),
		last_seen_version=CASE WHEN EXCLUDED.last_seen_version='' THEN server_errors.last_seen_version ELSE EXCLUDED.last_seen_version END`,
		incident.ID, incident.RequestID, incident.UserID, incident.Kind, incident.Severity, incident.Message, incident.Details, incident.Fingerprint, s.Version)
	return err
}

func (s *Store) ListIncidents(ctx context.Context, status string, limit, offset int) ([]model.Incident, int, error) {
	limit, offset = clampPage(limit, offset)
	var total int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM server_errors WHERE $1='' OR status=$1`, status).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT id::text,request_id,user_id::text,kind,severity,message,details,status,notes,occurred_at,updated_at,resolved_at,resolved_by::text,
		fingerprint,occurrence_count,first_occurred_at,last_occurred_at,first_seen_version,last_seen_version
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
			&incident.Fingerprint, &incident.OccurrenceCount, &incident.FirstOccurredAt, &incident.LastOccurredAt, &incident.FirstSeenVersion, &incident.LastSeenVersion); err != nil {
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
		fingerprint,occurrence_count,first_occurred_at,last_occurred_at,first_seen_version,last_seen_version`,
		id, actorID, status, notes).Scan(&incident.ID, &incident.RequestID, &incident.UserID, &incident.Kind, &incident.Severity, &incident.Message,
		&incident.Details, &incident.Status, &incident.Notes, &incident.OccurredAt, &incident.UpdatedAt, &incident.ResolvedAt, &incident.ResolvedBy,
		&incident.Fingerprint, &incident.OccurrenceCount, &incident.FirstOccurredAt, &incident.LastOccurredAt, &incident.FirstSeenVersion, &incident.LastSeenVersion)
	return incident, mapNotFound(err)
}

var secretPattern = regexp.MustCompile(`(?i)(bearer\s+|api[_-]?key["'=:\s]+|password["'=:\s]+|secret["'=:\s]+)([^\s,"}]+)`)

// An identifier changes on every request, and a fingerprint that keeps one is a
// fingerprint that never groups: five decks refused for the same reason opened
// five incidents, each headed by its own UUID. A UUID is matched whole because
// its middle groups are four characters long and neither half of the rule
// below reaches them.
var changingTokenPattern = regexp.MustCompile(
	`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b|` +
		`\b[0-9a-f]{8,}\b|\b\d{4,}\b`)

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

// UsageDay is what a deployment did in one day.
type UsageDay struct {
	Day       string `json:"day"`
	Generated int    `json:"generated"`
	Failed    int    `json:"failed"`
	// MedianSeconds is how long the middle generation took that day, and zero
	// when the day holds none that finished. An average is pulled around by one
	// slow deck; the middle one is what a day felt like.
	//
	// It is fractional on purpose: the built-in writer answers in under a
	// second, and rounding that to a whole number told an operator their decks
	// take "0초", which is a number nobody can act on.
	MedianSeconds float64 `json:"medianSeconds"`
	// SlowestSeconds is the longest one that day. On a self-hosted model the
	// middle deck is written by the built-in writer in hundredths of a second
	// and says nothing about what the model cost: the slow one does.
	SlowestSeconds float64 `json:"slowestSeconds"`
}

// UsageCount is one thing and how much of it: a person, a design, a reason.
type UsageCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
	// Detail carries what the name alone does not say — an owner's address, a
	// design's palette — so a screen can show who without guessing.
	Detail string `json:"detail,omitempty"`
}

// Usage is what this deployment has been doing, over the days it was asked
// about.
//
// The overview says what is true now: how many decks exist, what is queued.
// Nothing said what a week looked like — how many decks were written, how many
// failed, how long they took, who asked for them — and on a self-hosted model
// that time is the cost of running the thing.
type Usage struct {
	Days      []UsageDay   `json:"days"`
	Owners    []UsageCount `json:"owners"`
	Designs   []UsageCount `json:"designs"`
	Failures  []UsageCount `json:"failures"`
	Generated int          `json:"generated"`
	Failed    int          `json:"failed"`
	// Timed is how many of those generations recorded how long they took. A
	// deck written before this deployment kept that has no duration, and a
	// median drawn from nothing is a number nobody should read.
	Timed int `json:"timed"`
	// What the three lists above do not name. Each of them is the busiest few,
	// and a list of the busiest few read as a full accounting: eight people
	// shown against four and a half thousand decks, adding to less, with nothing
	// saying where the rest went. A screen that shows a part of something has to
	// say it is a part.
	OwnersOther   int `json:"ownersOther"`
	DesignsOther  int `json:"designsOther"`
	FailuresOther int `json:"failuresOther"`
}

// ReadUsage counts what happened over the last days, one row per day.
func (s *Store) ReadUsage(ctx context.Context, days int) (Usage, error) {
	if days < 1 {
		days = 7
	}
	if days > 180 {
		days = 180
	}
	usage := Usage{Days: []UsageDay{}, Owners: []UsageCount{}, Designs: []UsageCount{}, Failures: []UsageCount{}}
	rows, err := s.Pool.Query(ctx, `
		WITH span AS (SELECT generate_series((now() - ($1::int - 1) * interval '1 day')::date, now()::date, interval '1 day')::date AS day),
		made AS (
			SELECT created_at::date AS day, status,
				CASE WHEN generation_started_at IS NOT NULL AND generation_ended_at IS NOT NULL
					THEN extract(epoch FROM generation_ended_at - generation_started_at) END AS seconds
			FROM presentations
			WHERE deleted_at IS NULL AND created_at >= (now() - ($1::int - 1) * interval '1 day')::date
		)
		SELECT span.day::text,
			count(made.day),
			count(*) FILTER (WHERE made.status='failed'),
			COALESCE(percentile_cont(0.5) WITHIN GROUP (ORDER BY made.seconds), 0),
			COALESCE(max(made.seconds), 0)
		FROM span LEFT JOIN made ON made.day = span.day
		GROUP BY span.day ORDER BY span.day`, days)
	if err != nil {
		return usage, err
	}
	defer rows.Close()
	for rows.Next() {
		var day UsageDay
		var median, slowest float64
		if err := rows.Scan(&day.Day, &day.Generated, &day.Failed, &median, &slowest); err != nil {
			return usage, err
		}
		// Rounded the same way, or the two disagree with each other: a day whose
		// slowest deck took 0.04s reported "가장 오래 0초" beside "중앙 0.01초",
		// which says the middle deck outlasted the slowest one.
		day.MedianSeconds = math.Round(median*100) / 100
		day.SlowestSeconds = math.Round(slowest*100) / 100
		usage.Generated += day.Generated
		usage.Failed += day.Failed
		usage.Days = append(usage.Days, day)
	}
	if err := rows.Err(); err != nil {
		return usage, err
	}

	since := `p.deleted_at IS NULL AND p.created_at >= (now() - ($1::int - 1) * interval '1 day')::date`
	usage.Owners, err = s.usageCounts(ctx, `SELECT COALESCE(NULLIF(u.name,''), NULLIF(u.email,''), '알 수 없는 사용자'),
		COALESCE(u.email,''), count(*) FROM presentations p LEFT JOIN users u ON u.id = p.owner_id
		WHERE `+since+` GROUP BY 1,2 ORDER BY 3 DESC LIMIT 8`, days)
	if err != nil {
		return usage, err
	}
	usage.Designs, err = s.usageCounts(ctx, `SELECT COALESCE(NULLIF(t.name,''), NULLIF(p.theme,''), '지정 없음'),
		COALESCE(p.theme,''), count(*) FROM presentations p LEFT JOIN templates t ON t.id = p.template_id
		WHERE `+since+` GROUP BY 1,2 ORDER BY 3 DESC LIMIT 8`, days)
	if err != nil {
		return usage, err
	}
	// Why a deck did not come out, in the words its author was given. Two decks
	// that failed the same way are one thing to fix.
	usage.Failures, err = s.usageCounts(ctx, `SELECT COALESCE(NULLIF(left(p.error_message, 80),''), '이유가 기록되지 않았습니다'),
		'', count(*) FROM presentations p WHERE `+since+` AND p.status='failed'
		GROUP BY 1 ORDER BY 3 DESC LIMIT 6`, days)
	if err != nil {
		return usage, err
	}
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM presentations p
		WHERE `+since+` AND p.generation_started_at IS NOT NULL AND p.generation_ended_at IS NOT NULL`,
		days).Scan(&usage.Timed); err != nil {
		return usage, err
	}
	// The lists are the busiest few over exactly the same window the totals are
	// counted over, so what they leave out is the difference.
	usage.OwnersOther = leftOutOf(usage.Generated, usage.Owners)
	usage.DesignsOther = leftOutOf(usage.Generated, usage.Designs)
	usage.FailuresOther = leftOutOf(usage.Failed, usage.Failures)
	return usage, nil
}

// leftOutOf is how much of a total the listed few do not account for.
func leftOutOf(total int, listed []UsageCount) int {
	for _, one := range listed {
		total -= one.Count
	}
	if total < 0 {
		return 0
	}
	return total
}

func (s *Store) usageCounts(ctx context.Context, query string, days int) ([]UsageCount, error) {
	rows, err := s.Pool.Query(ctx, query, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := make([]UsageCount, 0, 8)
	for rows.Next() {
		var one UsageCount
		if err := rows.Scan(&one.Name, &one.Detail, &one.Count); err != nil {
			return nil, err
		}
		counts = append(counts, one)
	}
	return counts, rows.Err()
}

// TidyItem is one kind of thing that has accumulated, counted and dated.
type TidyItem struct {
	Kind string `json:"kind"`
	// Count is how many there are, Bytes what they take where that is known.
	Count int   `json:"count"`
	Bytes int64 `json:"bytes,omitempty"`
	// Oldest is when the oldest of them was left, and empty when there are none.
	Oldest string `json:"oldest,omitempty"`
}

// TidyPreview is what a retention policy would be deciding about.
//
// Nothing here is deleted, and nothing here proposes a rule: what to keep and
// for how long is somebody's decision, and it cannot be made without knowing
// what has accumulated. A deployment that has been running a year holds decks
// somebody binned in March and images no deck has ever drawn, and neither shows
// anywhere.
type TidyPreview struct {
	Items []TidyItem `json:"items"`
}

// ReadTidyPreview counts what is sitting in this deployment and going nowhere.
func (s *Store) ReadTidyPreview(ctx context.Context) (TidyPreview, error) {
	preview := TidyPreview{Items: []TidyItem{}}
	for _, ask := range []struct {
		kind  string
		query string
	}{
		{"trashed", `SELECT count(*), 0::bigint, COALESCE(min(deleted_at)::date::text,'')
			FROM presentations WHERE deleted_at IS NOT NULL`},
		{"failedOldDecks", `SELECT count(*), 0::bigint, COALESCE(min(updated_at)::date::text,'')
			FROM presentations WHERE deleted_at IS NULL AND status='failed' AND updated_at < now()-interval '30 days'`},
		{"untouchedDrafts", `SELECT count(*), 0::bigint, COALESCE(min(updated_at)::date::text,'')
			FROM presentations WHERE deleted_at IS NULL AND status='draft' AND updated_at < now()-interval '90 days'`},
		{"expiredLinks", `SELECT count(*), 0::bigint, COALESCE(min(expires_at)::date::text,'')
			FROM presentation_shares WHERE revoked_at IS NULL AND expires_at IS NOT NULL AND expires_at <= now()`},
		// An image no deck draws is not necessarily an image nobody wants: one
		// uploaded this morning has not been placed yet. The older ones are
		// counted apart so a decision can be made about them and not about
		// somebody's morning.
		{"unusedImages", `SELECT count(*), COALESCE(sum(a.size_bytes),0)::bigint, COALESCE(min(a.created_at)::date::text,'')
			FROM assets a WHERE NOT EXISTS (SELECT 1 FROM asset_usage u WHERE u.asset_id = a.id)`},
		{"unusedImagesOverAMonth", `SELECT count(*), COALESCE(sum(a.size_bytes),0)::bigint, COALESCE(min(a.created_at)::date::text,'')
			FROM assets a WHERE a.created_at < now()-interval '30 days'
			AND NOT EXISTS (SELECT 1 FROM asset_usage u WHERE u.asset_id = a.id)`},
		// The two that grow with every day the deployment is used rather than
		// with anything anybody forgot to tidy. Nothing else on this screen was
		// larger than either of them on a deployment a week old, and neither
		// appeared: an operator deciding how long to keep things was deciding
		// without the two biggest numbers.
		//
		// Their size is asked of the database rather than summed from a column,
		// because what a row costs is the row, its indexes and its toasted text.
		{"deckRevisions", `SELECT count(*), pg_total_relation_size('presentation_revisions')::bigint,
			COALESCE(min(created_at)::date::text,'') FROM presentation_revisions`},
		{"auditHistory", `SELECT count(*), pg_total_relation_size('audit_logs')::bigint,
			COALESCE(min(created_at)::date::text,'') FROM audit_logs`},
	} {
		item := TidyItem{Kind: ask.kind}
		if err := s.Pool.QueryRow(ctx, ask.query).Scan(&item.Count, &item.Bytes, &item.Oldest); err != nil {
			return preview, err
		}
		preview.Items = append(preview.Items, item)
	}
	return preview, nil
}

// SchemaState is how far the database has been brought up to date.
type SchemaState struct {
	Applied int `json:"applied"`
	Latest  int `json:"latest"`
}

// ReadSchemaState reads the migration ledger. A deployment whose image is newer
// than its database is a deployment about to behave strangely, and at a closed
// site nobody can look it up.
func (s *Store) ReadSchemaState(ctx context.Context) (SchemaState, error) {
	var state SchemaState
	err := s.Pool.QueryRow(ctx, `SELECT count(*), COALESCE(max(version),0) FROM schema_migrations`).
		Scan(&state.Applied, &state.Latest)
	return state, err
}
