package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hkjang/ptium/server/internal/model"
)

// MaximumSnippetBytes bounds one saved slide. A slide's source is a few hundred
// characters; anything past this is a deck, not a slide.
const MaximumSnippetBytes = 16 << 10

// SnippetInput is a slide being saved for reuse.
type SnippetInput struct {
	Name   string
	Source string
	Role   string
	Tags   []string
}

// SnippetQuery narrows someone's saved slides the same four ways their images
// are narrowed, because it is the same act: finding the one they mean.
type SnippetQuery struct {
	Search   string
	Tag      string
	Favorite bool
	// Sort is "recent" (newest, the default), "lastUsed", "used" or "name".
	Sort   string
	Limit  int
	Offset int
}

const snippetColumns = `s.id::text,s.owner_id::text,s.name,s.source,s.role,s.tags,
	(f.ref_id IS NOT NULL),s.use_count,s.last_used_at,s.created_at,s.updated_at`

const snippetFrom = ` FROM snippets s
	LEFT JOIN favorites f ON f.owner_id=s.owner_id AND f.kind='snippet' AND f.ref_id=s.id`

func snippetScan(snippet *model.Snippet) []any {
	return []any{&snippet.ID, &snippet.OwnerID, &snippet.Name, &snippet.Source, &snippet.Role,
		&snippet.Tags, &snippet.Favorite, &snippet.UseCount, &snippet.LastUsed,
		&snippet.CreatedAt, &snippet.UpdatedAt}
}

// CreateSnippet saves a slide under a name. Saving again under the same name
// replaces it, which is how someone updates the company introduction.
func (s *Store) CreateSnippet(ctx context.Context, ownerID string, in SnippetInput) (model.Snippet, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return model.Snippet{}, errors.New("a saved slide needs a name")
	}
	if len([]rune(name)) > 120 {
		return model.Snippet{}, errors.New("that name is too long for a saved slide")
	}
	source := strings.TrimSpace(in.Source)
	if source == "" {
		return model.Snippet{}, errors.New("there is nothing on this slide to save")
	}
	if len(source) > MaximumSnippetBytes {
		return model.Snippet{}, fmt.Errorf("a saved slide may be at most %d bytes of source", MaximumSnippetBytes)
	}
	role := strings.TrimSpace(in.Role)
	if role == "" {
		role = "content"
	}
	var id string
	err := s.Pool.QueryRow(ctx, `INSERT INTO snippets(id,owner_id,name,source,role,tags)
		VALUES($1,$2,$3,$4,$5,$6)
		ON CONFLICT (owner_id,lower(name)) DO UPDATE SET
			source=EXCLUDED.source,role=EXCLUDED.role,tags=EXCLUDED.tags,updated_at=now()
		RETURNING id::text`,
		newID(), ownerID, name, source, role, normalizeTags(in.Tags)).Scan(&id)
	if err != nil {
		return model.Snippet{}, err
	}
	return s.GetSnippet(ctx, id, ownerID)
}

// ListSnippets returns a person's saved slides.
// LibrarySnippets is every slide an owner has registered, for generation to
// look through before writing its own version of one.
//
// It is not a page. The listing endpoint caps a page at a hundred, and reusing
// it here meant that an owner with more than a hundred saved slides had the
// rest invisible to generation — and since the order is most-used first, the
// invisible ones were always the newly saved. A slide someone registers today
// has to be usable today.
func (s *Store) LibrarySnippets(ctx context.Context, ownerID string) ([]model.Snippet, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+snippetColumns+snippetFrom+
		` WHERE s.owner_id=$1 ORDER BY s.use_count DESC, s.updated_at DESC LIMIT 2000`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var snippets []model.Snippet
	for rows.Next() {
		var snippet model.Snippet
		if err := rows.Scan(snippetScan(&snippet)...); err != nil {
			return nil, err
		}
		snippets = append(snippets, snippet)
	}
	return snippets, rows.Err()
}

func (s *Store) ListSnippets(ctx context.Context, ownerID string, query SnippetQuery) ([]model.Snippet, int, error) {
	limit, offset := clampPage(query.Limit, query.Offset)
	where := ` WHERE s.owner_id=$1`
	args := []any{ownerID}
	if search := strings.TrimSpace(query.Search); search != "" {
		args = append(args, "%"+strings.ToLower(search)+"%")
		where += fmt.Sprintf(` AND (lower(s.name) LIKE $%d OR lower(s.source) LIKE $%d
			OR EXISTS (SELECT 1 FROM unnest(s.tags) t WHERE lower(t) LIKE $%d))`, len(args), len(args), len(args))
	}
	if tag := strings.TrimSpace(query.Tag); tag != "" {
		args = append(args, tag)
		where += fmt.Sprintf(` AND EXISTS (SELECT 1 FROM unnest(s.tags) t WHERE lower(t)=lower($%d))`, len(args))
	}
	if query.Favorite {
		where += ` AND f.ref_id IS NOT NULL`
	}
	var total int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*)`+snippetFrom+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	order := ` ORDER BY (f.ref_id IS NOT NULL) DESC, `
	switch query.Sort {
	case "used":
		order += `s.use_count DESC, s.updated_at DESC`
	case "lastUsed":
		order += `s.last_used_at DESC NULLS LAST, s.updated_at DESC`
	case "name":
		order += `lower(s.name)`
	default:
		order += `s.updated_at DESC`
	}
	args = append(args, limit, offset)
	rows, err := s.Pool.Query(ctx, `SELECT `+snippetColumns+snippetFrom+where+order+
		fmt.Sprintf(` LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := make([]model.Snippet, 0)
	for rows.Next() {
		var snippet model.Snippet
		if err := rows.Scan(snippetScan(&snippet)...); err != nil {
			return nil, 0, err
		}
		result = append(result, snippet)
	}
	return result, total, rows.Err()
}

// SnippetTags lists the words this person files saved slides under.
func (s *Store) SnippetTags(ctx context.Context, ownerID string) ([]model.AssetTag, error) {
	rows, err := s.Pool.Query(ctx, `SELECT t, count(*)::int FROM snippets s, unnest(s.tags) t
		WHERE s.owner_id=$1 GROUP BY t ORDER BY count(*) DESC, lower(t) LIMIT 40`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.AssetTag, 0)
	for rows.Next() {
		var tag model.AssetTag
		if err := rows.Scan(&tag.Name, &tag.Count); err != nil {
			return nil, err
		}
		result = append(result, tag)
	}
	return result, rows.Err()
}

func (s *Store) GetSnippet(ctx context.Context, id, ownerID string) (model.Snippet, error) {
	var snippet model.Snippet
	err := s.Pool.QueryRow(ctx, `SELECT `+snippetColumns+snippetFrom+
		` WHERE s.id=$1 AND s.owner_id=$2`, id, ownerID).Scan(snippetScan(&snippet)...)
	if err != nil {
		return model.Snippet{}, mapNotFound(err)
	}
	return snippet, nil
}

// SnippetPatch is what can be changed about a saved slide. A nil field is left
// alone.
type SnippetPatch struct {
	Name   *string
	Source *string
	Tags   *[]string
}

func (s *Store) UpdateSnippet(ctx context.Context, id, ownerID string, patch SnippetPatch) (model.Snippet, error) {
	sets := []string{"updated_at=now()"}
	args := []any{id, ownerID}
	if patch.Name != nil {
		name := strings.TrimSpace(*patch.Name)
		if name == "" || len([]rune(name)) > 120 {
			return model.Snippet{}, errors.New("a saved slide needs a name of at most 120 characters")
		}
		args = append(args, name)
		sets = append(sets, fmt.Sprintf("name=$%d", len(args)))
	}
	if patch.Source != nil {
		source := strings.TrimSpace(*patch.Source)
		if source == "" || len(source) > MaximumSnippetBytes {
			return model.Snippet{}, errors.New("a saved slide needs source text within the size limit")
		}
		args = append(args, source)
		sets = append(sets, fmt.Sprintf("source=$%d", len(args)))
	}
	if patch.Tags != nil {
		args = append(args, normalizeTags(*patch.Tags))
		sets = append(sets, fmt.Sprintf("tags=$%d", len(args)))
	}
	command, err := s.Pool.Exec(ctx, `UPDATE snippets SET `+strings.Join(sets, ",")+
		` WHERE id=$1 AND owner_id=$2`, args...)
	if err != nil {
		if strings.Contains(err.Error(), "snippets_owner_name_idx") {
			return model.Snippet{}, fmt.Errorf("%w: another saved slide already has that name", ErrConflict)
		}
		return model.Snippet{}, err
	}
	if command.RowsAffected() == 0 {
		return model.Snippet{}, ErrNotFound
	}
	return s.GetSnippet(ctx, id, ownerID)
}

func (s *Store) DeleteSnippet(ctx context.Context, id, ownerID string) error {
	command, err := s.Pool.Exec(ctx, `DELETE FROM snippets WHERE id=$1 AND owner_id=$2`, id, ownerID)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkSnippetUsed records an insertion.
//
// A counter is honest here, unlike for images: inserting is a deliberate act,
// and once inserted the slide is an ordinary slide with nothing left pointing
// back at what it came from.
func (s *Store) MarkSnippetUsed(ctx context.Context, id, ownerID string) {
	_, _ = s.Pool.Exec(ctx, `UPDATE snippets SET use_count=use_count+1,last_used_at=now()
		WHERE id=$1 AND owner_id=$2`, id, ownerID)
}
