package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
		`COALESCE((SELECT t.name FROM templates t WHERE t.id=` + prefix + `template_id),'')`
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
	limit, offset = clampPage(limit, offset)
	where := "WHERE owner_id=$1"
	args := []any{ownerID, limit, offset}
	if admin && ownerID == "" {
		where = ""
		args = []any{limit, offset}
	}
	var total int
	countSQL := `SELECT count(*) FROM presentations ` + where
	countArgs := args[:1]
	if where == "" {
		countArgs = nil
	}
	if err := s.Pool.QueryRow(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}
	projection := `SELECT ` + presentationColumns("presentations") + `,
		(SELECT count(*)::int FROM slides s WHERE s.presentation_id=presentations.id) FROM presentations `
	query := projection + where + ` ORDER BY presentations.updated_at DESC LIMIT $2 OFFSET $3`
	if where == "" {
		query = projection + ` ORDER BY presentations.updated_at DESC LIMIT $1 OFFSET $2`
	}
	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := make([]model.Presentation, 0)
	for rows.Next() {
		var p model.Presentation
		if err := rows.Scan(append(presentationScan(&p), &p.SlideCount)...); err != nil {
			return nil, 0, err
		}
		result = append(result, p)
	}
	return result, total, rows.Err()
}

func (s *Store) GetPresentation(ctx context.Context, id, ownerID string, admin bool) (model.Presentation, error) {
	query := `SELECT ` + presentationColumns("presentations") + ` FROM presentations WHERE id=$1`
	args := []any{id}
	if !admin {
		query += ` AND owner_id=$2`
		args = append(args, ownerID)
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
	query := `UPDATE presentations SET title=$2,prompt=$3,theme=$4,language=$5,audience=$6,tone=$7,requested_slide_count=$8,template_id=$9,updated_at=now() WHERE id=$1`
	args := []any{id, in.Title, in.Prompt, in.Theme, in.Language, in.Audience, in.Tone, in.SlideCount, in.TemplateID}
	if !admin {
		query += ` AND owner_id=$10`
		args = append(args, ownerID)
	}
	query += ` RETURNING ` + presentationColumns("presentations")
	var p model.Presentation
	err := s.Pool.QueryRow(ctx, query, args...).Scan(presentationScan(&p)...)
	return p, mapNotFound(err)
}

// UpdatePresentationWithSlides atomically updates presentation metadata and,
// when slides is non-nil, replaces the editable slide sequence.
func (s *Store) UpdatePresentationWithSlides(ctx context.Context, id, ownerID string, admin bool, in PresentationInput, slides *[]model.Slide) (model.Presentation, error) {
	if slides != nil && (len(*slides) == 0 || len(*slides) > 50) {
		return model.Presentation{}, errors.New("slides must contain between 1 and 50 items")
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return model.Presentation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	ownerQuery := `SELECT owner_id::text FROM presentations WHERE id=$1`
	ownerArgs := []any{id}
	if !admin {
		ownerQuery += ` AND owner_id=$2`
		ownerArgs = append(ownerArgs, ownerID)
	}
	ownerQuery += ` FOR UPDATE`
	var actualOwner string
	if err := tx.QueryRow(ctx, ownerQuery, ownerArgs...).Scan(&actualOwner); err != nil {
		return model.Presentation{}, mapNotFound(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE presentations SET title=$2,prompt=$3,theme=$4,language=$5,audience=$6,tone=$7,requested_slide_count=$8,template_id=$9,updated_at=now() WHERE id=$1`,
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
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Presentation{}, err
	}
	return s.GetPresentation(ctx, id, actualOwner, true)
}

func (s *Store) DeletePresentation(ctx context.Context, id, ownerID string, admin bool) error {
	query := `DELETE FROM presentations WHERE id=$1`
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

func (s *Store) QueueGeneration(ctx context.Context, id, ownerID string, admin bool, maximumSlides int) (model.Presentation, error) {
	if maximumSlides < 1 || maximumSlides > 50 {
		maximumSlides = 50
	}
	query := `UPDATE presentations SET status='queued',error_message='',generation_started_at=NULL,generation_ended_at=NULL,updated_at=now() WHERE id=$1 AND requested_slide_count<=$2`
	args := []any{id, maximumSlides}
	if !admin {
		query += ` AND owner_id=$3`
		args = append(args, ownerID)
	}
	query += ` RETURNING ` + presentationColumns("presentations")
	var p model.Presentation
	err := s.Pool.QueryRow(ctx, query, args...).Scan(presentationScan(&p)...)
	if !errors.Is(err, pgx.ErrNoRows) {
		return p, err
	}
	checkQuery := `SELECT requested_slide_count FROM presentations WHERE id=$1`
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

func (s *Store) ClaimGeneration(ctx context.Context) (model.Presentation, error) {
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.Presentation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Reclaim work abandoned by a crashed process after ten minutes.
	_, _ = tx.Exec(ctx, `UPDATE presentations SET status='queued',updated_at=now() WHERE status='generating' AND generation_started_at < now()-interval '10 minutes'`)
	var p model.Presentation
	err = tx.QueryRow(ctx, `SELECT `+presentationColumns("presentations")+`
		FROM presentations WHERE status='queued' ORDER BY updated_at FOR UPDATE OF presentations SKIP LOCKED LIMIT 1`).Scan(presentationScan(&p)...)
	if err != nil {
		return model.Presentation{}, mapNotFound(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE presentations SET status='generating',generation_started_at=now(),updated_at=now() WHERE id=$1`, p.ID); err != nil {
		return model.Presentation{}, err
	}
	p.Status = "generating"
	now := time.Now().UTC()
	p.GenerationStartedAt = &now
	if err := tx.Commit(ctx); err != nil {
		return model.Presentation{}, err
	}
	return p, nil
}

func (s *Store) CompleteGeneration(ctx context.Context, id string, outline json.RawMessage, slides []model.Slide) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
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
	result, err := tx.Exec(ctx, `UPDATE presentations SET status='completed',outline=$2,error_message='',generation_ended_at=now(),updated_at=now() WHERE id=$1 AND status='generating'`, id, outline)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return errors.New("presentation generation state changed")
	}
	return tx.Commit(ctx)
}

func (s *Store) FailGeneration(ctx context.Context, id, message string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE presentations SET status='failed',error_message=$2,generation_ended_at=now(),updated_at=now() WHERE id=$1`, id, message)
	return err
}

func presentationScan(p *model.Presentation) []any {
	return []any{&p.ID, &p.OwnerID, &p.Title, &p.Prompt, &p.Status, &p.Theme, &p.Language, &p.Audience, &p.Tone,
		&p.RequestedSlideCount, &p.Outline, &p.ErrorMessage, &p.CreatedAt, &p.UpdatedAt,
		&p.GenerationStartedAt, &p.GenerationEndedAt, &p.TemplateID, &p.TemplateName}
}
