package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hkjang/ptium/server/internal/model"
	"github.com/jackc/pgx/v5"
)

type PresentationInput struct {
	Title      string
	Prompt     string
	Theme      string
	Language   string
	Audience   string
	Tone       string
	SlideCount int
	TemplateID *string
}

// presentationListColumns is the projection for a list of decks.
//
// It is presentationColumns without the two heavy fields: the deck's own source
// and its outline. A list shows a title, a status and a date, and sending the
// full text of every deck to draw that made the front page of an account with
// six hundred decks a megabyte of JSON — most of it text nothing on the page
// reads. A deck that is opened is fetched on its own, with everything.
func presentationListColumns(prefix string) string {
	if prefix != "" {
		prefix += "."
	}
	return prefix + `id::text,` + prefix + `owner_id::text,` + prefix + `title,` + prefix + `prompt,` + prefix + `status,` +
		prefix + `theme,` + prefix + `language,` + prefix + `audience,` + prefix + `tone,` + prefix + `requested_slide_count,` +
		`NULL::jsonb,` + prefix + `error_message,` + prefix + `created_at,` + prefix + `updated_at,` +
		prefix + `generation_started_at,` + prefix + `generation_ended_at,` + prefix + `template_id::text,` +
		`''::text,` +
		`COALESCE((SELECT t.name FROM templates t WHERE t.id=` + prefix + `template_id),''),` +
		`COALESCE(` + prefix + `generation_notes,'[]'::jsonb),` +
		prefix + `version,` + prefix + `deleted_at,` +
		`COALESCE(` + prefix + `rewrite_instruction,''),` +
		`COALESCE(` + prefix + `generation_stage,'')`
}

// presentationColumns builds the projection used by every presentation query.
// The prefix is the table name or alias the query uses.
func presentationColumns(prefix string) string {
	if prefix != "" {
		prefix += "."
	}
	return prefix + `id::text,` + prefix + `owner_id::text,` + prefix + `title,` + prefix + `prompt,` + prefix + `status,` +
		prefix + `theme,` + prefix + `language,` + prefix + `audience,` + prefix + `tone,` + prefix + `requested_slide_count,` +
		prefix + `outline,` + prefix + `error_message,` + prefix + `created_at,` + prefix + `updated_at,` +
		prefix + `generation_started_at,` + prefix + `generation_ended_at,` + prefix + `template_id::text,` +
		`COALESCE(` + prefix + `source,''),` +
		`COALESCE((SELECT t.name FROM templates t WHERE t.id=` + prefix + `template_id),''),` +
		`COALESCE(` + prefix + `generation_notes,'[]'::jsonb),` +
		prefix + `version,` + prefix + `deleted_at,` +
		`COALESCE(` + prefix + `rewrite_instruction,''),` +
		`COALESCE(` + prefix + `generation_stage,'')`
}

func (s *Store) CreatePresentation(ctx context.Context, ownerID string, in PresentationInput) (model.Presentation, error) {
	var p model.Presentation
	err := s.Pool.QueryRow(ctx, `INSERT INTO presentations(owner_id,title,prompt,theme,language,audience,tone,requested_slide_count,template_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING `+presentationColumns("presentations"),
		ownerID, in.Title, in.Prompt, in.Theme, in.Language, in.Audience, in.Tone, in.SlideCount, in.TemplateID).Scan(presentationScan(&p)...)
	return p, err
}

// CreateAndQueuePresentation inserts a new deck directly in queued state so a
// failed queue transition cannot leave a draft behind.
func (s *Store) CreateAndQueuePresentation(ctx context.Context, ownerID string, in PresentationInput) (model.Presentation, error) {
	var p model.Presentation
	err := s.Pool.QueryRow(ctx, `INSERT INTO presentations(owner_id,title,prompt,status,theme,language,audience,tone,requested_slide_count,template_id)
		VALUES($1,$2,$3,'queued',$4,$5,$6,$7,$8,$9) RETURNING `+presentationColumns("presentations"),
		ownerID, in.Title, in.Prompt, in.Theme, in.Language, in.Audience, in.Tone, in.SlideCount, in.TemplateID).Scan(presentationScan(&p)...)
	return p, err
}

func (s *Store) ListPresentations(ctx context.Context, ownerID string, admin bool, limit, offset int) ([]model.Presentation, int, error) {
	return s.listPresentations(ctx, ownerID, admin, false, "", limit, offset)
}

// SearchPresentations is ListPresentations narrowed to the decks whose title or
// brief contains what was typed.
//
// Searching belongs here rather than in the browser: the front page used to
// fetch every deck the account had in order to filter a list of titles, which
// is fine at ten decks and a megabyte of JSON at six hundred.
func (s *Store) SearchPresentations(ctx context.Context, ownerID string, admin, deleted bool,
	search string, limit, offset int) ([]model.Presentation, int, error) {
	return s.listPresentations(ctx, ownerID, admin, deleted, search, limit, offset)
}

// ListDeletedPresentations returns only items in the caller's recycle bin.
func (s *Store) ListDeletedPresentations(ctx context.Context, ownerID string, admin bool, limit, offset int) ([]model.Presentation, int, error) {
	return s.listPresentations(ctx, ownerID, admin, true, "", limit, offset)
}

func (s *Store) listPresentations(ctx context.Context, ownerID string, admin, deleted bool,
	search string, limit, offset int) ([]model.Presentation, int, error) {
	limit, offset = clampPage(limit, offset)
	deletedClause := "deleted_at IS NULL"
	if deleted {
		deletedClause = "deleted_at IS NOT NULL"
	}
	where := "WHERE owner_id=$1 AND " + deletedClause
	filterArgs := []any{ownerID}
	if admin && ownerID == "" {
		where = "WHERE " + deletedClause
		filterArgs = nil
	}
	if wanted := strings.ToLower(strings.TrimSpace(search)); wanted != "" {
		filterArgs = append(filterArgs, likePattern(wanted))
		where += fmt.Sprintf(" AND (lower(title) LIKE $%d"+likeEscape+" OR lower(prompt) LIKE $%d"+likeEscape+")", len(filterArgs), len(filterArgs))
	}
	var total int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM presentations `+where, filterArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args := append(append([]any{}, filterArgs...), limit, offset)
	// What is waiting on a deck belongs beside its name. Reviewers leave remarks
	// on the link and nothing said so until the deck was opened, one at a time:
	// somebody with a dozen decks had no way to see which two were waiting on
	// them. Replies are not counted — a thread is one thing to deal with.
	projection := `SELECT ` + presentationListColumns("presentations") + `,
		(SELECT count(*)::int FROM slides s WHERE s.presentation_id=presentations.id),
		(SELECT count(*)::int FROM slide_comments c WHERE c.presentation_id=presentations.id
			AND c.parent_id IS NULL AND c.resolved_at IS NULL) FROM presentations `
	query := projection + where + fmt.Sprintf(` ORDER BY presentations.updated_at DESC LIMIT $%d OFFSET $%d`,
		len(args)-1, len(args))
	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := make([]model.Presentation, 0)
	for rows.Next() {
		var p model.Presentation
		if err := rows.Scan(append(presentationScan(&p), &p.SlideCount, &p.OpenComments)...); err != nil {
			return nil, 0, err
		}
		result = append(result, p)
	}
	return result, total, rows.Err()
}

func (s *Store) GetPresentation(ctx context.Context, id, ownerID string, admin bool) (model.Presentation, error) {
	return s.getPresentation(ctx, id, ownerID, admin, false)
}

func (s *Store) getPresentation(ctx context.Context, id, ownerID string, admin, includeDeleted bool) (model.Presentation, error) {
	query := `SELECT ` + presentationColumns("presentations") + ` FROM presentations WHERE id=$1`
	args := []any{id}
	if !admin {
		query += ` AND owner_id=$2`
		args = append(args, ownerID)
	}
	if !includeDeleted {
		query += ` AND deleted_at IS NULL`
	}
	var p model.Presentation
	if err := s.Pool.QueryRow(ctx, query, args...).Scan(presentationScan(&p)...); err != nil {
		return p, mapNotFound(err)
	}
	rows, err := s.Pool.Query(ctx, `SELECT id::text,presentation_id::text,position,title,subtitle,content,speaker_notes,layout,layout_id,created_at,updated_at
		FROM slides WHERE presentation_id=$1 ORDER BY position`, id)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	for rows.Next() {
		var slide model.Slide
		if err := rows.Scan(&slide.ID, &slide.PresentationID, &slide.Position, &slide.Title, &slide.Subtitle, &slide.Content, &slide.SpeakerNotes, &slide.Layout, &slide.LayoutID, &slide.CreatedAt, &slide.UpdatedAt); err != nil {
			return p, err
		}
		p.Slides = append(p.Slides, slide)
	}
	p.SlideCount = len(p.Slides)
	return p, rows.Err()
}

func (s *Store) UpdatePresentation(ctx context.Context, id, ownerID string, admin bool, in PresentationInput) (model.Presentation, error) {
	return s.UpdatePresentationWithSlides(ctx, id, ownerID, admin, in, nil, nil)
}

// UpdatePresentationWithSlides atomically updates presentation metadata and,
// when slides is non-nil, replaces the editable slide sequence.
func (s *Store) UpdatePresentationWithSlides(ctx context.Context, id, ownerID string, admin bool, in PresentationInput, slides *[]model.Slide, expectedVersion *int64) (model.Presentation, error) {
	if slides != nil && (len(*slides) == 0 || len(*slides) > 50) {
		return model.Presentation{}, errors.New("slides must contain between 1 and 50 items")
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return model.Presentation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	ownerQuery := `SELECT owner_id::text,version FROM presentations WHERE id=$1 AND deleted_at IS NULL`
	ownerArgs := []any{id}
	if !admin {
		ownerQuery += ` AND owner_id=$2`
		ownerArgs = append(ownerArgs, ownerID)
	}
	ownerQuery += ` FOR UPDATE`
	var actualOwner string
	var currentVersion int64
	if err := tx.QueryRow(ctx, ownerQuery, ownerArgs...).Scan(&actualOwner, &currentVersion); err != nil {
		return model.Presentation{}, mapNotFound(err)
	}
	if expectedVersion != nil && *expectedVersion != currentVersion {
		return model.Presentation{}, ErrConflict
	}
	if err := snapshotPresentationTx(ctx, tx, id, "edit", false); err != nil {
		return model.Presentation{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE presentations SET title=$2,prompt=$3,theme=$4,language=$5,audience=$6,tone=$7,requested_slide_count=$8,template_id=$9,version=version+1,updated_at=now() WHERE id=$1`,
		id, in.Title, in.Prompt, in.Theme, in.Language, in.Audience, in.Tone, in.SlideCount, in.TemplateID); err != nil {
		return model.Presentation{}, err
	}
	if slides != nil {
		if _, err := tx.Exec(ctx, `DELETE FROM slides WHERE presentation_id=$1`, id); err != nil {
			return model.Presentation{}, err
		}
		for index, slide := range *slides {
			if len(slide.Content) == 0 || !json.Valid(slide.Content) {
				return model.Presentation{}, fmt.Errorf("slide %d content must be valid JSON", index+1)
			}
			slideID := slide.ID
			if slideID == "" {
				slideID = newID()
			}
			if _, err := tx.Exec(ctx, `INSERT INTO slides(id,presentation_id,position,title,subtitle,content,speaker_notes,layout,layout_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
				slideID, id, index+1, slide.Title, slide.Subtitle, slide.Content, slide.SpeakerNotes, slide.Layout, slide.LayoutID); err != nil {
				return model.Presentation{}, fmt.Errorf("save slide %d: %w", index+1, err)
			}
		}
		if err := syncAssetUsageTx(ctx, tx, id); err != nil {
			return model.Presentation{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Presentation{}, err
	}
	return s.GetPresentation(ctx, id, actualOwner, true)
}

func (s *Store) DeletePresentation(ctx context.Context, id, ownerID string, admin bool) error {
	query := `UPDATE presentations SET deleted_at=now(),version=version+1,updated_at=now() WHERE id=$1 AND deleted_at IS NULL`
	args := []any{id}
	if !admin {
		query += ` AND owner_id=$2`
		args = append(args, ownerID)
	}
	result, err := s.Pool.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RestoreDeletedPresentation moves a deck out of the recycle bin without
// changing any of its content.
func (s *Store) RestoreDeletedPresentation(ctx context.Context, id, ownerID string, admin bool) (model.Presentation, error) {
	query := `UPDATE presentations SET deleted_at=NULL,version=version+1,updated_at=now() WHERE id=$1 AND deleted_at IS NOT NULL`
	args := []any{id}
	if !admin {
		query += ` AND owner_id=$2`
		args = append(args, ownerID)
	}
	query += ` RETURNING ` + presentationColumns("presentations")
	var restored model.Presentation
	if err := s.Pool.QueryRow(ctx, query, args...).Scan(presentationScan(&restored)...); err != nil {
		return model.Presentation{}, mapNotFound(err)
	}
	return s.GetPresentation(ctx, restored.ID, restored.OwnerID, true)
}

// EmptyTrash permanently deletes every deck already in the recycle bin, and
// answers with how many that was.
//
// One at a time is the only way there was, and a deployment that has been
// running for a while holds thousands: the trash was a room nobody could clear.
// Nothing here decides on anybody's behalf — this runs when a person asks for
// it, with the count in front of them — and the condition is the same one
// permanent deletion has always had, so a deck that was never deleted cannot be
// reached from here.
func (s *Store) EmptyTrash(ctx context.Context, ownerID string, admin bool) (int, error) {
	query := `DELETE FROM presentations WHERE deleted_at IS NOT NULL`
	var args []any
	if !admin {
		query += ` AND owner_id=$1`
		args = append(args, ownerID)
	}
	result, err := s.Pool.Exec(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return int(result.RowsAffected()), nil
}

// PermanentlyDeletePresentation is deliberately restricted to a deck that is
// already in the recycle bin, keeping the ordinary delete operation recoverable.
func (s *Store) PermanentlyDeletePresentation(ctx context.Context, id, ownerID string, admin bool) error {
	query := `DELETE FROM presentations WHERE id=$1 AND deleted_at IS NOT NULL`
	args := []any{id}
	if !admin {
		query += ` AND owner_id=$2`
		args = append(args, ownerID)
	}
	result, err := s.Pool.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DuplicatePresentation makes an independent copy. Slide IDs are regenerated,
// so future edits and revision restores can never affect the original deck.
func (s *Store) DuplicatePresentation(ctx context.Context, id, ownerID, title string) (model.Presentation, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return model.Presentation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	copyID := newID()
	var copied model.Presentation
	err = tx.QueryRow(ctx, `INSERT INTO presentations(
		id,owner_id,title,prompt,status,theme,language,audience,tone,requested_slide_count,outline,
		error_message,generation_started_at,generation_ended_at,template_id,source)
		SELECT $3,p.owner_id,$4,p.prompt,
			CASE WHEN EXISTS(SELECT 1 FROM slides s WHERE s.presentation_id=p.id) THEN 'completed' ELSE 'draft' END,
			p.theme,p.language,p.audience,p.tone,p.requested_slide_count,p.outline,'',NULL,NULL,p.template_id,p.source
		FROM presentations p WHERE p.id=$1 AND p.owner_id=$2 AND p.deleted_at IS NULL
		RETURNING `+presentationColumns("presentations"), id, ownerID, copyID, title).Scan(presentationScan(&copied)...)
	if err != nil {
		return model.Presentation{}, mapNotFound(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO slides(id,presentation_id,position,title,subtitle,content,speaker_notes,layout,layout_id)
		SELECT gen_random_uuid(),$2,position,title,subtitle,content,speaker_notes,layout,layout_id
		FROM slides WHERE presentation_id=$1 ORDER BY position`, id, copyID); err != nil {
		return model.Presentation{}, err
	}
	if err := syncAssetUsageTx(ctx, tx, copyID); err != nil {
		return model.Presentation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Presentation{}, err
	}
	return s.GetPresentation(ctx, copyID, ownerID, false)
}

// ListPresentationRevisions returns compact checkpoint metadata, newest first.
//
// Newest is decided by the deck's own version rather than by the clock. A
// checkpoint's version is the version it was taken at, and a deck's version only
// ever counts up, so ordering by it cannot be wrong. Wall-clock time can:
// 7 checkpoints of 2,795 decks in one workspace carry a created_at earlier than
// the checkpoint before them — a clock that stepped backwards under a
// virtualised host is enough — and the deck's history then lists the wrong
// checkpoint first. Restoring "the newest" gave back a deck from four changes
// ago, which is the one thing version history must never do.
func (s *Store) ListPresentationRevisions(ctx context.Context, id, ownerID string, limit, offset int) ([]model.PresentationRevision, int, error) {
	limit, offset = clampPage(limit, offset)
	var exists bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM presentations
		WHERE id=$1 AND owner_id=$2 AND deleted_at IS NULL)`, id, ownerID).Scan(&exists); err != nil {
		return nil, 0, err
	}
	if !exists {
		return nil, 0, ErrNotFound
	}
	var total int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM presentation_revisions r
		JOIN presentations p ON p.id=r.presentation_id
		WHERE r.presentation_id=$1 AND p.owner_id=$2 AND p.deleted_at IS NULL`, id, ownerID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT r.id::text,r.presentation_id::text,r.version,r.reason,r.title,r.slide_count,r.created_at
		FROM presentation_revisions r JOIN presentations p ON p.id=r.presentation_id
		WHERE r.presentation_id=$1 AND p.owner_id=$2 AND p.deleted_at IS NULL
		ORDER BY r.version DESC, r.created_at DESC LIMIT $3 OFFSET $4`, id, ownerID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := make([]model.PresentationRevision, 0)
	for rows.Next() {
		var revision model.PresentationRevision
		if err := rows.Scan(&revision.ID, &revision.PresentationID, &revision.Version, &revision.Reason,
			&revision.Title, &revision.SlideCount, &revision.CreatedAt); err != nil {
			return nil, 0, err
		}
		result = append(result, revision)
	}
	return result, total, rows.Err()
}

type presentationSnapshot struct {
	Title               string          `json:"title"`
	Prompt              string          `json:"prompt"`
	Theme               string          `json:"theme"`
	Language            string          `json:"language"`
	Audience            string          `json:"audience"`
	Tone                string          `json:"tone"`
	RequestedSlideCount int             `json:"requestedSlideCount"`
	TemplateID          *string         `json:"templateId"`
	Outline             json.RawMessage `json:"outline"`
	Source              string          `json:"source"`
	Slides              []model.Slide   `json:"slides"`
}

// snapshotPresentationTx records the state before a mutation. Ordinary
// autosaves are coalesced into five-minute editing checkpoints; important
// transitions pass force=true and always get their own checkpoint.
func snapshotPresentationTx(ctx context.Context, tx pgx.Tx, id, reason string, force bool) error {
	_, err := tx.Exec(ctx, `INSERT INTO presentation_revisions(presentation_id,owner_id,version,reason,title,slide_count,snapshot)
		SELECT p.id,p.owner_id,p.version,$2,p.title,
			(SELECT count(*)::int FROM slides count_slides WHERE count_slides.presentation_id=p.id),
			jsonb_build_object(
				'title',p.title,'prompt',p.prompt,'theme',p.theme,'language',p.language,
				'audience',p.audience,'tone',p.tone,'requestedSlideCount',p.requested_slide_count,
				'templateId',p.template_id,'outline',p.outline,'source',p.source,
				'slides',COALESCE((SELECT jsonb_agg(jsonb_build_object(
					'id',s.id::text,'position',s.position,'title',s.title,'subtitle',s.subtitle,
					'content',s.content,'speakerNotes',s.speaker_notes,'layout',s.layout,'layoutId',s.layout_id
				) ORDER BY s.position) FROM slides s WHERE s.presentation_id=p.id),'[]'::jsonb)
			)
		FROM presentations p
		WHERE p.id=$1 AND ($3 OR NOT EXISTS(
			SELECT 1 FROM presentation_revisions recent
			WHERE recent.presentation_id=p.id AND recent.reason='edit'
				AND recent.created_at > now()-interval '5 minutes'))
		ON CONFLICT(presentation_id,version) DO NOTHING`, id, reason, force)
	return err
}

// RestorePresentationRevision replaces the current deck atomically and first
// checkpoints it, so a restore can itself be undone.
// PresentationRevisionSlides is the deck as one version had it, for showing
// what changed since.
func (s *Store) PresentationRevisionSlides(ctx context.Context, id, revisionID, ownerID string) ([]model.Slide, error) {
	var raw json.RawMessage
	if err := s.Pool.QueryRow(ctx, `SELECT r.snapshot FROM presentation_revisions r
		JOIN presentations p ON p.id=r.presentation_id
		WHERE r.id=$1 AND r.presentation_id=$2 AND p.owner_id=$3 AND p.deleted_at IS NULL`,
		revisionID, id, ownerID).Scan(&raw); err != nil {
		return nil, mapNotFound(err)
	}
	var snapshot presentationSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return nil, err
	}
	return snapshot.Slides, nil
}

func (s *Store) RestorePresentationRevision(ctx context.Context, id, revisionID, ownerID string) (model.Presentation, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return model.Presentation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var actualOwner string
	if err := tx.QueryRow(ctx, `SELECT owner_id::text FROM presentations
		WHERE id=$1 AND owner_id=$2 AND deleted_at IS NULL FOR UPDATE`, id, ownerID).Scan(&actualOwner); err != nil {
		return model.Presentation{}, mapNotFound(err)
	}
	if err := snapshotPresentationTx(ctx, tx, id, "restore", true); err != nil {
		return model.Presentation{}, err
	}
	var raw json.RawMessage
	if err := tx.QueryRow(ctx, `SELECT snapshot FROM presentation_revisions
		WHERE id=$1 AND presentation_id=$2 AND owner_id=$3`, revisionID, id, ownerID).Scan(&raw); err != nil {
		return model.Presentation{}, mapNotFound(err)
	}
	var snapshot presentationSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return model.Presentation{}, fmt.Errorf("decode presentation revision: %w", err)
	}
	if snapshot.Title == "" || len(snapshot.Slides) > 50 {
		return model.Presentation{}, errors.New("presentation revision is not restorable")
	}
	if len(snapshot.Outline) == 0 || !json.Valid(snapshot.Outline) {
		snapshot.Outline = json.RawMessage(`[]`)
	}
	if snapshot.TemplateID != nil {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM templates WHERE id=$1)`, *snapshot.TemplateID).Scan(&exists); err != nil {
			return model.Presentation{}, err
		}
		if !exists {
			snapshot.TemplateID = nil
		}
	}
	status := "draft"
	if len(snapshot.Slides) > 0 {
		status = "completed"
	}
	if _, err := tx.Exec(ctx, `UPDATE presentations SET title=$2,prompt=$3,status=$4,theme=$5,language=$6,
		audience=$7,tone=$8,requested_slide_count=$9,template_id=$10,outline=$11,source=$12,
		error_message='',generation_started_at=NULL,generation_ended_at=NULL,deleted_at=NULL,version=version+1,updated_at=now()
		WHERE id=$1`, id, snapshot.Title, snapshot.Prompt, status, snapshot.Theme, snapshot.Language,
		snapshot.Audience, snapshot.Tone, snapshot.RequestedSlideCount, snapshot.TemplateID, snapshot.Outline, snapshot.Source); err != nil {
		return model.Presentation{}, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM slides WHERE presentation_id=$1`, id); err != nil {
		return model.Presentation{}, err
	}
	for index, slide := range snapshot.Slides {
		if len(slide.Content) == 0 || !json.Valid(slide.Content) {
			return model.Presentation{}, fmt.Errorf("revision slide %d has invalid content", index+1)
		}
		slideID := slide.ID
		if slideID == "" {
			slideID = newID()
		}
		if _, err := tx.Exec(ctx, `INSERT INTO slides(id,presentation_id,position,title,subtitle,content,speaker_notes,layout,layout_id)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, slideID, id, index+1, slide.Title, slide.Subtitle,
			slide.Content, slide.SpeakerNotes, slide.Layout, slide.LayoutID); err != nil {
			return model.Presentation{}, fmt.Errorf("restore revision slide %d: %w", index+1, err)
		}
	}
	if err := syncAssetUsageTx(ctx, tx, id); err != nil {
		return model.Presentation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Presentation{}, err
	}
	return s.GetPresentation(ctx, id, actualOwner, true)
}

func (s *Store) QueueGeneration(ctx context.Context, id, ownerID string, admin bool, maximumSlides int) (model.Presentation, error) {
	return s.QueueGenerationWith(ctx, id, ownerID, admin, maximumSlides, "")
}

// SetGenerationStage records which pass a generation is in, so the screen that
// waits can say more than "생성하고 있어요". It is written outside any
// transaction and read by a poll: a stage that arrives late is a screen a second
// behind, which is what a stage is for.
func (s *Store) SetGenerationStage(ctx context.Context, id, stage string) {
	_, _ = s.Pool.Exec(ctx, `UPDATE presentations SET generation_stage=$2 WHERE id=$1`, id, stage)
}

// QueueGenerationWith queues a deck with what the author asked for in their own
// words, which the worker reads when it picks the deck up. A rewrite that
// forgets the instruction between the click and the queue is a rewrite of
// something nobody asked about.
func (s *Store) QueueGenerationWith(ctx context.Context, id, ownerID string, admin bool, maximumSlides int,
	instruction string) (model.Presentation, error) {
	if maximumSlides < 1 || maximumSlides > 50 {
		maximumSlides = 50
	}
	query := `UPDATE presentations SET status='queued',error_message='',generation_started_at=NULL,generation_ended_at=NULL,generation_stage='',rewrite_instruction=$3,version=version+1,updated_at=now() WHERE id=$1 AND requested_slide_count<=$2 AND deleted_at IS NULL`
	args := []any{id, maximumSlides, strings.TrimSpace(instruction)}
	if !admin {
		query += ` AND owner_id=$4`
		args = append(args, ownerID)
	}
	query += ` RETURNING ` + presentationColumns("presentations")
	var p model.Presentation
	err := s.Pool.QueryRow(ctx, query, args...).Scan(presentationScan(&p)...)
	if !errors.Is(err, pgx.ErrNoRows) {
		return p, err
	}
	checkQuery := `SELECT requested_slide_count FROM presentations WHERE id=$1 AND deleted_at IS NULL`
	checkArgs := []any{id}
	if !admin {
		checkQuery += ` AND owner_id=$2`
		checkArgs = append(checkArgs, ownerID)
	}
	var requestedSlides int
	if checkErr := s.Pool.QueryRow(ctx, checkQuery, checkArgs...).Scan(&requestedSlides); checkErr != nil {
		return model.Presentation{}, mapNotFound(checkErr)
	}
	if requestedSlides > maximumSlides {
		return model.Presentation{}, ErrGenerationLimit
	}
	return model.Presentation{}, mapNotFound(err)
}

// GenerationSilence is how long a deck being written may say nothing before
// another worker may take it.
//
// It used to be ten minutes measured from the start, which is a guess about how
// long generation takes rather than a fact about whether anybody is still doing
// it. A self-hosted model answers in five minutes or more with thinking on, and
// a deployment may ask for up to ten repair passes on top — so a healthy deck
// was handed to a second worker while the first was still waiting on the model,
// and both wrote it. A worker says it is alive every half minute; three minutes
// of silence is a worker that is gone.
const GenerationSilence = 3 * time.Minute

// ClaimGeneration takes the next deck waiting to be written, and answers with
// the lease that says this worker is the one writing it.
func (s *Store) ClaimGeneration(ctx context.Context) (model.Presentation, string, error) {
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.Presentation{}, "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Reclaim work whose worker has stopped saying it is alive. A deck claimed
	// before this release has no heartbeat, so its start time answers for it.
	_, _ = tx.Exec(ctx, `UPDATE presentations SET status='queued',generation_stage='',generation_lease=NULL,updated_at=now()
		WHERE status='generating' AND deleted_at IS NULL
		AND COALESCE(generation_heartbeat_at, generation_started_at) < now()-$1::interval`,
		GenerationSilence.String())
	var p model.Presentation
	err = tx.QueryRow(ctx, `SELECT `+presentationColumns("presentations")+`
		FROM presentations WHERE status='queued' AND deleted_at IS NULL ORDER BY updated_at FOR UPDATE OF presentations SKIP LOCKED LIMIT 1`).Scan(presentationScan(&p)...)
	if err != nil {
		return model.Presentation{}, "", mapNotFound(err)
	}
	var lease string
	if err := tx.QueryRow(ctx, `UPDATE presentations SET status='generating',generation_started_at=now(),
		generation_heartbeat_at=now(),generation_lease=gen_random_uuid(),updated_at=now()
		WHERE id=$1 RETURNING generation_lease::text`, p.ID).Scan(&lease); err != nil {
		return model.Presentation{}, "", err
	}
	p.Status = "generating"
	now := time.Now().UTC()
	p.GenerationStartedAt = &now
	if err := tx.Commit(ctx); err != nil {
		return model.Presentation{}, "", err
	}
	return p, lease, nil
}

// HeartbeatGeneration is the worker holding this lease saying it is still
// writing the deck. It answers false when the lease is no longer theirs — the
// deck was stopped, requeued, or taken by somebody else — which is the worker's
// signal to stop rather than race whoever has it now.
func (s *Store) HeartbeatGeneration(ctx context.Context, id, lease string) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `UPDATE presentations SET generation_heartbeat_at=now()
		WHERE id=$1 AND generation_lease=$2::uuid AND status='generating' AND deleted_at IS NULL`, id, lease)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// CompleteGeneration stores a finished deck: its source, its outline and its
// slides, in one transaction.
func (s *Store) CompleteGeneration(ctx context.Context, id, lease string, outline json.RawMessage, slides []model.Slide,
	source string, notes []string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := snapshotPresentationTx(ctx, tx, id, "generation", true); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM slides WHERE presentation_id=$1`, id); err != nil {
		return err
	}
	for i, slide := range slides {
		if len(slide.Content) == 0 {
			slide.Content = json.RawMessage(`{}`)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO slides(id,presentation_id,position,title,subtitle,content,speaker_notes,layout,layout_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			newID(), id, i+1, slide.Title, slide.Subtitle, slide.Content, slide.SpeakerNotes, slide.Layout, slide.LayoutID); err != nil {
			return fmt.Errorf("insert slide %d: %w", i+1, err)
		}
	}
	if err := syncAssetUsageTx(ctx, tx, id); err != nil {
		return err
	}
	recorded, err := json.Marshal(notes)
	if err != nil || len(notes) == 0 {
		recorded = []byte(`[]`)
	}
	// Only the worker that holds this deck's lease may finish it. Without that,
	// an attempt taken over by another worker still wrote its answer over the
	// live one when it eventually came back.
	result, err := tx.Exec(ctx, `UPDATE presentations SET status='completed',outline=$2,source=$3,
		generation_notes=$4,error_message='',generation_ended_at=now(),generation_stage='',generation_lease=NULL,
		version=version+1,updated_at=now()
		WHERE id=$1 AND status='generating' AND deleted_at IS NULL AND generation_lease=$5::uuid`,
		id, outline, source, recorded, lease)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return errors.New("presentation generation state changed")
	}
	return tx.Commit(ctx)
}

// ReplaceSlidesFromSource stores hand-edited deck source and the slides it
// compiles to. The two always move together: source that is stored without its
// slides would describe a deck nobody can see.
func (s *Store) ReplaceSlidesFromSource(ctx context.Context, id, ownerID string, source string,
	outline json.RawMessage, slides []model.Slide, expectedVersion *int64) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var currentVersion int64
	if err := tx.QueryRow(ctx, `SELECT version FROM presentations
		WHERE id=$1 AND owner_id=$2 AND deleted_at IS NULL FOR UPDATE`, id, ownerID).Scan(&currentVersion); err != nil {
		return mapNotFound(err)
	}
	if expectedVersion != nil && *expectedVersion != currentVersion {
		return ErrConflict
	}
	if err := snapshotPresentationTx(ctx, tx, id, "source", true); err != nil {
		return err
	}
	// A deck with slides in it is not a draft. Someone who wrote their deck as
	// source, or brought one in from a file, has a finished deck; leaving it
	// labelled "초안" in their library describes how it was made rather than what
	// it is. A generation still in flight keeps its own status.
	result, err := tx.Exec(ctx, `UPDATE presentations SET source=$3,outline=$4,
		status=CASE WHEN status IN ('draft','completed') THEN 'completed' ELSE status END,
		version=version+1,updated_at=now()
		WHERE id=$1 AND owner_id=$2 AND deleted_at IS NULL`, id, ownerID, source, outline)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `DELETE FROM slides WHERE presentation_id=$1`, id); err != nil {
		return err
	}
	for index, slide := range slides {
		if len(slide.Content) == 0 {
			slide.Content = json.RawMessage(`{}`)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO slides(id,presentation_id,position,title,subtitle,content,speaker_notes,layout,layout_id)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			newID(), id, index+1, slide.Title, slide.Subtitle, slide.Content, slide.SpeakerNotes, slide.Layout, slide.LayoutID); err != nil {
			return fmt.Errorf("insert slide %d: %w", index+1, err)
		}
	}
	if err := syncAssetUsageTx(ctx, tx, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// FailGeneration is a worker saying that the deck it was writing did not come
// out, in the words its author reads.
//
// It applies to a deck that is still being written and to nothing else. Two
// things go wrong otherwise. An operator stops a deck and gives a reason, and
// the worker finishing a moment later overwrites it with "try again" — so the
// author is told to retry something somebody deliberately stopped. And an
// operator pushes a stuck deck back into the queue, and a worker still holding
// the previous attempt fails the fresh one before anybody picks it up.
func (s *Store) FailGeneration(ctx context.Context, id, lease, message string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE presentations SET status='failed',error_message=$2,generation_ended_at=now(),
		generation_stage='',generation_lease=NULL,version=version+1,updated_at=now()
		WHERE id=$1 AND status='generating' AND deleted_at IS NULL AND generation_lease=$3::uuid`, id, message, lease)
	return err
}

// StopGeneration is an operator standing a deck down, with a reason its author
// reads. Unlike a worker's failure it also takes a deck that is only waiting:
// stopping the queue is most of the point of being able to see it.
// It answers whether it stopped anything. A deck that finished in the moment
// between an operator reading the queue and pressing the button is not stopped,
// and saying nothing there is telling them it was.
func (s *Store) StopGeneration(ctx context.Context, id, reason string) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `UPDATE presentations SET status='failed',error_message=$2,generation_ended_at=now(),
		generation_stage='',version=version+1,updated_at=now()
		WHERE id=$1 AND status IN ('queued','generating') AND deleted_at IS NULL`, id, reason)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func presentationScan(p *model.Presentation) []any {
	return []any{&p.ID, &p.OwnerID, &p.Title, &p.Prompt, &p.Status, &p.Theme, &p.Language, &p.Audience, &p.Tone,
		&p.RequestedSlideCount, &p.Outline, &p.ErrorMessage, &p.CreatedAt, &p.UpdatedAt,
		&p.GenerationStartedAt, &p.GenerationEndedAt, &p.TemplateID, &p.Source, &p.TemplateName,
		&p.GenerationNotes, &p.Version, &p.DeletedAt, &p.RewriteInstruction, &p.GenerationStage}
}
