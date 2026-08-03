package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

// ErrTemplateInvalid marks an upload that is not a usable PowerPoint template.
var ErrTemplateInvalid = errors.New("template is not a usable PowerPoint file")

// TemplateInput carries the values a caller may set on a template.
type TemplateInput struct {
	Name        string
	Description string
	Filename    string
	Scope       string
	Data        []byte
}

const templateColumns = `id::text,owner_id::text,name,description,filename,kind,scope,palette_key,size_bytes,checksum,manifest,created_at,updated_at`

// CreateTemplate stores an uploaded template together with the manifest that
// describes its layouts.
func (s *Store) CreateTemplate(ctx context.Context, ownerID string, in TemplateInput) (model.Template, error) {
	manifest, err := analyzeTemplate(in.Data)
	if err != nil {
		return model.Template{}, err
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return model.Template{}, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = strings.TrimSpace(strings.TrimSuffix(in.Filename, ".pptx"))
	}
	if name == "" {
		name = "새 템플릿"
	}
	scope := in.Scope
	if scope != "shared" {
		scope = "private"
	}
	checksum := sha256.Sum256(in.Data)
	var template model.Template
	err = s.Pool.QueryRow(ctx, `INSERT INTO templates(owner_id,name,description,filename,kind,scope,size_bytes,checksum,manifest,data)
		VALUES($1,$2,$3,$4,'uploaded',$5,$6,$7,$8,$9) RETURNING `+templateColumns,
		ownerID, name, in.Description, in.Filename, scope, len(in.Data), hex.EncodeToString(checksum[:]), encoded, in.Data).Scan(templateScan(&template)...)
	if err != nil {
		return model.Template{}, err
	}
	decorateTemplate(&template)
	return template, nil
}

// UpdateTemplate changes the editable metadata of a template.
func (s *Store) UpdateTemplate(ctx context.Context, id, ownerID string, admin bool, in TemplateInput) (model.Template, error) {
	scope := in.Scope
	if scope != "shared" {
		scope = "private"
	}
	query := `UPDATE templates SET name=$2,description=$3,scope=$4,updated_at=now() WHERE id=$1 AND kind='uploaded'`
	args := []any{id, strings.TrimSpace(in.Name), in.Description, scope}
	if !admin {
		query += ` AND owner_id=$5`
		args = append(args, ownerID)
	}
	query += ` RETURNING ` + templateColumns
	var template model.Template
	if err := s.Pool.QueryRow(ctx, query, args...).Scan(templateScan(&template)...); err != nil {
		return model.Template{}, mapNotFound(err)
	}
	decorateTemplate(&template)
	return template, nil
}

// ListTemplates returns the templates a user may generate with: their own
// uploads plus every shared and built-in design.
func (s *Store) ListTemplates(ctx context.Context, ownerID string, limit, offset int) ([]model.Template, int, error) {
	limit, offset = clampPage(limit, offset)
	const visible = `(owner_id=$1 OR scope='shared' OR kind='builtin')`
	var total int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM templates WHERE `+visible, ownerID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT `+templateColumns+`,
		(SELECT count(*)::int FROM presentations p WHERE p.template_id=templates.id)
		FROM templates WHERE `+visible+`
		ORDER BY kind='builtin', (owner_id=$1) DESC, updated_at DESC LIMIT $2 OFFSET $3`, ownerID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := make([]model.Template, 0)
	for rows.Next() {
		var template model.Template
		if err := rows.Scan(append(templateScan(&template), &template.UsageCount)...); err != nil {
			return nil, 0, err
		}
		decorateTemplate(&template)
		result = append(result, template)
	}
	return result, total, rows.Err()
}

// GetTemplate reads one template's metadata and manifest.
func (s *Store) GetTemplate(ctx context.Context, id, ownerID string, admin bool) (model.Template, error) {
	query := `SELECT ` + templateColumns + ` FROM templates WHERE id=$1`
	args := []any{id}
	if !admin {
		query += ` AND (owner_id=$2 OR scope='shared' OR kind='builtin')`
		args = append(args, ownerID)
	}
	var template model.Template
	if err := s.Pool.QueryRow(ctx, query, args...).Scan(templateScan(&template)...); err != nil {
		return model.Template{}, mapNotFound(err)
	}
	decorateTemplate(&template)
	return template, nil
}

// TemplateData returns the stored package bytes alongside its manifest.
func (s *Store) TemplateData(ctx context.Context, id, ownerID string, admin bool) ([]byte, model.Template, error) {
	query := `SELECT ` + templateColumns + `,data FROM templates WHERE id=$1`
	args := []any{id}
	if !admin {
		query += ` AND (owner_id=$2 OR scope='shared' OR kind='builtin')`
		args = append(args, ownerID)
	}
	var template model.Template
	var data []byte
	if err := s.Pool.QueryRow(ctx, query, args...).Scan(append(templateScan(&template), &data)...); err != nil {
		return nil, model.Template{}, mapNotFound(err)
	}
	decorateTemplate(&template)
	return data, template, nil
}

// DeleteTemplate removes an uploaded template. Built-in designs cannot be
// deleted because presentations may still reference them.
func (s *Store) DeleteTemplate(ctx context.Context, id, ownerID string, admin bool) error {
	query := `DELETE FROM templates WHERE id=$1 AND kind='uploaded'`
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

// EnsureBuiltinTemplates seeds — and keeps current — the designs Ptium ships
// with, so a fresh install can generate a polished deck immediately and an
// offline deployment never needs to download anything.
func (s *Store) EnsureBuiltinTemplates(ctx context.Context) error {
	for _, key := range pptx.BuiltinPaletteKeys() {
		palette := pptx.LookupBuiltinPalette(key)
		data, err := pptx.BuiltinTemplate(key)
		if err != nil {
			return fmt.Errorf("build %s template: %w", key, err)
		}
		manifest, err := analyzeTemplate(data)
		if err != nil {
			return fmt.Errorf("analyze %s template: %w", key, err)
		}
		encoded, err := json.Marshal(manifest)
		if err != nil {
			return err
		}
		checksum := sha256.Sum256(data)
		digest := hex.EncodeToString(checksum[:])
		if _, err := s.Pool.Exec(ctx, `INSERT INTO templates(owner_id,name,description,filename,kind,scope,palette_key,size_bytes,checksum,manifest,data)
			VALUES(NULL,$1,$2,$3,'builtin','shared',$4,$5,$6,$7,$8)
			ON CONFLICT (palette_key) WHERE kind='builtin' DO UPDATE SET
				name=EXCLUDED.name,description=EXCLUDED.description,size_bytes=EXCLUDED.size_bytes,
				checksum=EXCLUDED.checksum,manifest=EXCLUDED.manifest,data=EXCLUDED.data,updated_at=now()
			WHERE templates.checksum <> EXCLUDED.checksum
			   OR COALESCE((templates.manifest->>'version')::int, 0) < $9`,
			"Ptium "+palette.Name, builtinDescription(palette), key+".pptx", key, len(data), digest, encoded, data,
			pptx.ManifestVersion); err != nil {
			return fmt.Errorf("seed %s template: %w", key, err)
		}
	}
	return nil
}

// DefaultTemplateID resolves the template a new deck should use when the
// caller did not choose one: the user's most recent upload if they have any,
// otherwise the built-in design matching the requested theme.
func (s *Store) DefaultTemplateID(ctx context.Context, ownerID, theme string) (string, error) {
	var id string
	err := s.Pool.QueryRow(ctx, `SELECT id::text FROM templates WHERE palette_key=$1 AND kind='builtin'`, strings.ToLower(strings.TrimSpace(theme))).Scan(&id)
	if err == nil {
		return id, nil
	}
	err = s.Pool.QueryRow(ctx, `SELECT id::text FROM templates WHERE kind='builtin' ORDER BY palette_key LIMIT 1`).Scan(&id)
	return id, mapNotFound(err)
}

func builtinDescription(palette pptx.BuiltinPalette) string {
	return fmt.Sprintf("Ptium 기본 디자인 · 16:9 · 표지, 구역, 본문, 2단, 비교, 인용, 이미지, 마무리 레이아웃 (%s)", palette.Name)
}

func analyzeTemplate(data []byte) (pptx.Manifest, error) {
	_, manifest, err := pptx.AnalyzeBytes(data)
	if err != nil {
		return pptx.Manifest{}, fmt.Errorf("%w: %s", ErrTemplateInvalid, err.Error())
	}
	return manifest, nil
}

func templateScan(template *model.Template) []any {
	return []any{&template.ID, &template.OwnerID, &template.Name, &template.Description, &template.Filename,
		&template.Kind, &template.Scope, &template.PaletteKey, &template.SizeBytes, &template.Checksum,
		&template.Manifest, &template.CreatedAt, &template.UpdatedAt}
}

// decorateTemplate fills the summary fields the API exposes so clients do not
// have to parse the full manifest just to render a card.
func decorateTemplate(template *model.Template) {
	var manifest struct {
		AspectRatio string `json:"aspectRatio"`
		Layouts     []struct {
			ID string `json:"id"`
		} `json:"layouts"`
	}
	if json.Unmarshal(template.Manifest, &manifest) != nil {
		return
	}
	template.AspectRatio = manifest.AspectRatio
	template.LayoutCount = len(manifest.Layouts)
}
