package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"  // registers the GIF decoder
	_ "image/jpeg" // registers the JPEG decoder
	_ "image/png"  // registers the PNG decoder
	"strings"

	"github.com/hkjang/ptium/server/internal/model"
	"github.com/jackc/pgx/v5"
)

// ErrAssetUnsupported reports an upload Ptium will not place on a slide.
var ErrAssetUnsupported = errors.New("unsupported image")

// MaximumAssetBytes bounds one image. A slide picture beyond this is a
// photograph nobody downscaled, and it would bloat every export that uses it.
const MaximumAssetBytes = 16 << 20

// assetContentTypes are the formats PowerPoint reads and Ptium can therefore
// place without converting.
var assetContentTypes = map[string]string{
	"image/png":     "png",
	"image/jpeg":    "jpeg",
	"image/gif":     "gif",
	"image/svg+xml": "svg",
}

// AssetInput is an image being stored.
type AssetInput struct {
	Name        string
	ContentType string
	Data        []byte
}

// CreateAsset stores an image for a user. A second upload of the same name
// replaces it, which is what someone re-exporting a logo expects.
func (s *Store) CreateAsset(ctx context.Context, ownerID string, in AssetInput) (model.Asset, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return model.Asset{}, fmt.Errorf("%w: an image needs a name", ErrValidation)
	}
	if len(in.Data) == 0 {
		return model.Asset{}, fmt.Errorf("%w: the file is empty", ErrValidation)
	}
	if len(in.Data) > MaximumAssetBytes {
		return model.Asset{}, fmt.Errorf("%w: larger than %d bytes", ErrAssetUnsupported, MaximumAssetBytes)
	}
	contentType := detectAssetType(in.ContentType, in.Data)
	if _, ok := assetContentTypes[contentType]; !ok {
		return model.Asset{}, fmt.Errorf("%w: %s", ErrAssetUnsupported, contentType)
	}
	width, height := imageSize(in.Data)
	digest := sha256.Sum256(in.Data)
	checksum := hex.EncodeToString(digest[:])

	// The same picture, uploaded again. A library fills up with "logo.png",
	// "logo (1).png", "스크린샷 2026-08-21.png" and every one of them is the same
	// bytes; handing back the one already there keeps the library a library.
	// Uploading it under a name that already exists is a replacement and is
	// handled below, so only a new name reaches this.
	if existing, err := s.assetByChecksum(ctx, ownerID, checksum); err == nil {
		existing.Reused = true
		return existing, nil
	} else if !errors.Is(err, ErrNotFound) {
		return model.Asset{}, err
	}

	// With a volume the row keeps the description and the volume keeps the
	// picture, so the column is left null. The row is written first because the
	// id it returns is the name the bytes are filed under — a replaced image
	// keeps the id it already had.
	column := in.Data
	if s.Blobs != nil {
		column = nil
	}
	transaction, err := s.Pool.Begin(ctx)
	if err != nil {
		return model.Asset{}, err
	}
	defer transaction.Rollback(context.WithoutCancel(ctx))

	var asset model.Asset
	err = transaction.QueryRow(ctx, `INSERT INTO assets(id,owner_id,name,content_type,size_bytes,width,height,checksum,data)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (owner_id,lower(name)) DO UPDATE SET
			content_type=EXCLUDED.content_type,size_bytes=EXCLUDED.size_bytes,
			width=EXCLUDED.width,height=EXCLUDED.height,checksum=EXCLUDED.checksum,data=EXCLUDED.data
		RETURNING id::text,owner_id::text,name,content_type,size_bytes,width,height,checksum,created_at`,
		newID(), ownerID, name, contentType, len(in.Data), width, height,
		checksum, column).Scan(&asset.ID, &asset.OwnerID, &asset.Name,
		&asset.ContentType, &asset.SizeBytes, &asset.Width, &asset.Height, &asset.Checksum, &asset.CreatedAt)
	if err != nil {
		return model.Asset{}, err
	}
	if s.Blobs != nil {
		// A full or read-only volume must fail the upload, not record an image
		// nobody can open. The row is rolled back with it.
		if err := s.Blobs.Put(asset.ID, in.Data); err != nil {
			return model.Asset{}, fmt.Errorf("store image bytes: %w", err)
		}
	}
	if err := transaction.Commit(ctx); err != nil {
		return model.Asset{}, err
	}
	return asset, nil
}

// AssetQuery narrows a person's image library.
//
// A library people actually keep things in stops being a list very quickly. The
// four ways anyone looks for a picture — I starred it, I used it recently, I use
// it constantly, I remember its name — are all here, and none of them is a
// different screen.
type AssetQuery struct {
	Search   string
	Tag      string
	Favorite bool
	// Sort is one of "recent" (newest upload, the default), "used" (most decks),
	// "lastUsed" (most recently placed) or "name".
	Sort   string
	Limit  int
	Offset int
}

// assetColumns reads an image with everything a person sorts and filters by.
// The counts come from the decks that place it rather than from a counter, so
// they cannot drift.
const assetColumns = `a.id::text,a.owner_id::text,a.name,a.content_type,a.size_bytes,a.width,a.height,
	a.checksum,a.tags,a.created_at,
	(f.ref_id IS NOT NULL),
	(SELECT count(*)::int FROM asset_usage u JOIN presentations p ON p.id=u.presentation_id
		WHERE u.asset_id=a.id AND p.deleted_at IS NULL),
	(SELECT max(u.updated_at) FROM asset_usage u JOIN presentations p ON p.id=u.presentation_id
		WHERE u.asset_id=a.id AND p.deleted_at IS NULL)`

const assetFrom = ` FROM assets a
	LEFT JOIN favorites f ON f.owner_id=a.owner_id AND f.kind='asset' AND f.ref_id=a.id`

func assetScan(asset *model.Asset) []any {
	return []any{&asset.ID, &asset.OwnerID, &asset.Name, &asset.ContentType, &asset.SizeBytes,
		&asset.Width, &asset.Height, &asset.Checksum, &asset.Tags, &asset.CreatedAt,
		&asset.Favorite, &asset.DeckCount, &asset.LastUsed}
}

// ListAssets returns a user's images without their bytes.
func (s *Store) ListAssets(ctx context.Context, ownerID string, query AssetQuery) ([]model.Asset, int, error) {
	limit, offset := clampPage(query.Limit, query.Offset)
	where := ` WHERE a.owner_id=$1`
	args := []any{ownerID}
	if search := strings.TrimSpace(query.Search); search != "" {
		args = append(args, "%"+strings.ToLower(search)+"%")
		where += fmt.Sprintf(` AND (lower(a.name) LIKE $%d OR EXISTS (SELECT 1 FROM unnest(a.tags) t WHERE lower(t) LIKE $%d))`, len(args), len(args))
	}
	if tag := strings.TrimSpace(query.Tag); tag != "" {
		args = append(args, tag)
		where += fmt.Sprintf(` AND EXISTS (SELECT 1 FROM unnest(a.tags) t WHERE lower(t)=lower($%d))`, len(args))
	}
	if query.Favorite {
		where += ` AND f.ref_id IS NOT NULL`
	}
	var total int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*)`+assetFrom+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	// Starred images lead every ordering. Someone who pinned an image did so to
	// stop scrolling for it.
	order := ` ORDER BY (f.ref_id IS NOT NULL) DESC, `
	switch query.Sort {
	case "used":
		order += `2 DESC, a.created_at DESC`
	case "lastUsed":
		order += `3 DESC NULLS LAST, a.created_at DESC`
	case "name":
		order += `lower(a.name)`
	default:
		order += `a.created_at DESC`
	}
	// The sort keys are the counted columns, referenced by position so the
	// subqueries are not written twice.
	order = strings.Replace(order, `2 DESC`, `(SELECT count(*) FROM asset_usage u JOIN presentations p ON p.id=u.presentation_id WHERE u.asset_id=a.id AND p.deleted_at IS NULL) DESC`, 1)
	order = strings.Replace(order, `3 DESC NULLS LAST`, `(SELECT max(u.updated_at) FROM asset_usage u JOIN presentations p ON p.id=u.presentation_id WHERE u.asset_id=a.id AND p.deleted_at IS NULL) DESC NULLS LAST`, 1)
	args = append(args, limit, offset)
	rows, err := s.Pool.Query(ctx, `SELECT `+assetColumns+assetFrom+where+order+
		fmt.Sprintf(` LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := make([]model.Asset, 0)
	for rows.Next() {
		var asset model.Asset
		if err := rows.Scan(assetScan(&asset)...); err != nil {
			return nil, 0, err
		}
		result = append(result, asset)
	}
	return result, total, rows.Err()
}

// AssetTags lists the tags this person has used, with how many images carry
// each, so the library offers what they already write rather than a free field.
func (s *Store) AssetTags(ctx context.Context, ownerID string) ([]model.AssetTag, error) {
	rows, err := s.Pool.Query(ctx, `SELECT t, count(*)::int FROM assets a, unnest(a.tags) t
		WHERE a.owner_id=$1 GROUP BY t ORDER BY count(*) DESC, lower(t) LIMIT 40`, ownerID)
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

// GetAsset reads one image's description.
func (s *Store) GetAsset(ctx context.Context, id, ownerID string) (model.Asset, error) {
	var asset model.Asset
	err := s.Pool.QueryRow(ctx, `SELECT `+assetColumns+assetFrom+` WHERE a.id=$1 AND a.owner_id=$2`,
		id, ownerID).Scan(assetScan(&asset)...)
	if err != nil {
		return model.Asset{}, mapNotFound(err)
	}
	return asset, nil
}

// assetByChecksum finds an image this person has already uploaded, byte for byte.
func (s *Store) assetByChecksum(ctx context.Context, ownerID, checksum string) (model.Asset, error) {
	var asset model.Asset
	err := s.Pool.QueryRow(ctx, `SELECT `+assetColumns+assetFrom+`
		WHERE a.owner_id=$1 AND a.checksum=$2 ORDER BY a.created_at LIMIT 1`,
		ownerID, checksum).Scan(assetScan(&asset)...)
	if err != nil {
		return model.Asset{}, mapNotFound(err)
	}
	return asset, nil
}

// AssetPatch is what a person can change about an image after uploading it. A
// nil field is left alone.
type AssetPatch struct {
	Name *string
	Tags *[]string
}

// UpdateAsset renames or retags an image.
//
// The name is how deck source refers to a picture, so renaming one is a real
// edit: it must stay unique per person, and the decks that wrote the old name in
// their source keep working because they hold the id, not the name.
func (s *Store) UpdateAsset(ctx context.Context, id, ownerID string, patch AssetPatch) (model.Asset, error) {
	sets := []string{}
	args := []any{id, ownerID}
	if patch.Name != nil {
		name := strings.TrimSpace(*patch.Name)
		if name == "" {
			return model.Asset{}, fmt.Errorf("%w: an image needs a name", ErrValidation)
		}
		if len([]rune(name)) > 160 {
			return model.Asset{}, fmt.Errorf("%w: that name is too long for an image", ErrValidation)
		}
		args = append(args, name)
		sets = append(sets, fmt.Sprintf("name=$%d", len(args)))
	}
	if patch.Tags != nil {
		tags := normalizeTags(*patch.Tags)
		args = append(args, tags)
		sets = append(sets, fmt.Sprintf("tags=$%d", len(args)))
	}
	if len(sets) == 0 {
		return s.GetAsset(ctx, id, ownerID)
	}
	command, err := s.Pool.Exec(ctx, `UPDATE assets SET `+strings.Join(sets, ",")+
		` WHERE id=$1 AND owner_id=$2`, args...)
	if err != nil {
		if strings.Contains(err.Error(), "assets_owner_name_idx") {
			return model.Asset{}, fmt.Errorf("%w: another image already has that name", ErrConflict)
		}
		return model.Asset{}, err
	}
	if command.RowsAffected() == 0 {
		return model.Asset{}, ErrNotFound
	}
	return s.GetAsset(ctx, id, ownerID)
}

// normalizeTags keeps a tag list short, trimmed and free of repeats, so the tag
// bar stays a way of finding things.
func normalizeTags(tags []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" || len([]rune(tag)) > 24 {
			continue
		}
		key := strings.ToLower(tag)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, tag)
		if len(result) == 8 {
			break
		}
	}
	return result
}

// AssetData returns an image's bytes, from wherever this deployment keeps them.
//
// Both places are read. A deployment that mounts a volume today still has every
// image uploaded before it did, and those rows keep working — and are moved onto
// the volume the first time they are read, so the database empties itself as the
// pictures get used rather than in one migration nobody scheduled.
func (s *Store) AssetData(ctx context.Context, id, ownerID string) ([]byte, model.Asset, error) {
	var asset model.Asset
	var data []byte
	err := s.Pool.QueryRow(ctx, `SELECT id::text,owner_id::text,name,content_type,size_bytes,width,height,checksum,created_at,data
		FROM assets WHERE id=$1 AND owner_id=$2`, id, ownerID).Scan(&asset.ID, &asset.OwnerID, &asset.Name,
		&asset.ContentType, &asset.SizeBytes, &asset.Width, &asset.Height, &asset.Checksum, &asset.CreatedAt, &data)
	if err != nil {
		return nil, model.Asset{}, mapNotFound(err)
	}
	if s.Blobs == nil {
		if len(data) == 0 {
			return nil, model.Asset{}, fmt.Errorf("%s: %w", asset.Name, ErrBlobMissing)
		}
		return data, asset, nil
	}
	stored, err := s.Blobs.Get(asset.ID)
	switch {
	case err == nil:
		return stored, asset, nil
	case !errors.Is(err, ErrBlobMissing):
		return nil, model.Asset{}, err
	case len(data) == 0:
		// Neither place has it. Usually the volume was swapped or never restored.
		return nil, model.Asset{}, fmt.Errorf("%s: %w", asset.Name, ErrBlobMissing)
	}
	s.moveToVolume(ctx, asset.ID, data)
	return data, asset, nil
}

// moveToVolume writes an older database-held image onto the volume and clears
// the column. Failing is not worth an error to the reader — they asked for the
// picture, they got the picture — so it is left for the next read to retry.
func (s *Store) moveToVolume(ctx context.Context, id string, data []byte) {
	if err := s.Blobs.Put(id, data); err != nil {
		return
	}
	_, _ = s.Pool.Exec(ctx, `UPDATE assets SET data=NULL WHERE id=$1`, id)
}

// ResolveAsset finds an image by id or by the name its owner gave it, which is
// what deck source refers to.
func (s *Store) ResolveAsset(ctx context.Context, ownerID, reference string) (model.Asset, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return model.Asset{}, ErrNotFound
	}
	var asset model.Asset
	err := s.Pool.QueryRow(ctx, `SELECT id::text,owner_id::text,name,content_type,size_bytes,width,height,checksum,created_at
		FROM assets WHERE owner_id=$1 AND (id::text=$2 OR lower(name)=lower($2))
		ORDER BY (id::text=$2) DESC LIMIT 1`, ownerID, reference).Scan(&asset.ID, &asset.OwnerID,
		&asset.Name, &asset.ContentType, &asset.SizeBytes, &asset.Width, &asset.Height, &asset.Checksum, &asset.CreatedAt)
	if err != nil {
		return model.Asset{}, mapNotFound(err)
	}
	return asset, nil
}

// DeleteAsset removes an image.
func (s *Store) DeleteAsset(ctx context.Context, id, ownerID string) error {
	result, err := s.Pool.Exec(ctx, `DELETE FROM assets WHERE id=$1 AND owner_id=$2`, id, ownerID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	if s.Blobs != nil {
		// The row is gone either way; a file left behind on the volume would be
		// storage nobody can reach, so removing it is worth reporting.
		if err := s.Blobs.Delete(id); err != nil {
			return fmt.Errorf("remove image bytes: %w", err)
		}
	}
	return nil
}

// detectAssetType prefers what the bytes say over what the upload claimed.
func detectAssetType(claimed string, data []byte) string {
	switch {
	case len(data) > 8 && string(data[1:4]) == "PNG":
		return "image/png"
	case len(data) > 3 && data[0] == 0xFF && data[1] == 0xD8:
		return "image/jpeg"
	case len(data) > 6 && string(data[:3]) == "GIF":
		return "image/gif"
	case bytes.Contains(data[:min(len(data), 512)], []byte("<svg")):
		return "image/svg+xml"
	}
	return strings.ToLower(strings.TrimSpace(strings.Split(claimed, ";")[0]))
}

// imageSize reads an image's pixel dimensions, which decide how it is cropped
// into a slide's frame. An SVG has none, and is stretched to fit instead.
func imageSize(data []byte) (int, int) {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0
	}
	return config.Width, config.Height
}

var _ = pgx.ErrNoRows

// syncAssetUsageTx records which images a deck places, from the slides as they
// have just been written.
//
// It reads the deck back rather than taking the slides it was handed, because
// five different paths write slides — an edit, a duplicate, a restore, a
// generation, an applied source — and a fact this useful should not depend on
// each of them remembering to pass the same thing.
func syncAssetUsageTx(ctx context.Context, tx pgx.Tx, presentationID string) error {
	rows, err := tx.Query(ctx, `SELECT content FROM slides WHERE presentation_id=$1`, presentationID)
	if err != nil {
		return err
	}
	used := map[string]bool{}
	for rows.Next() {
		var content []byte
		if err := rows.Scan(&content); err != nil {
			rows.Close()
			return err
		}
		for _, id := range assetsInContent(content) {
			used[id] = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	ids := make([]string, 0, len(used))
	for id := range used {
		ids = append(ids, id)
	}
	// Rows for images the deck no longer places go first, so an image removed
	// from the last deck that used it stops counting immediately.
	if _, err := tx.Exec(ctx, `DELETE FROM asset_usage WHERE presentation_id=$1 AND NOT (asset_id = ANY($2::uuid[]))`,
		presentationID, ids); err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	// An image can be deleted while a deck still names it, so a row that no
	// longer points at an asset is skipped rather than failing the save.
	_, err = tx.Exec(ctx, `INSERT INTO asset_usage(asset_id,presentation_id,updated_at)
		SELECT a.id,$1,now() FROM assets a WHERE a.id = ANY($2::uuid[])
		ON CONFLICT (asset_id,presentation_id) DO UPDATE SET updated_at=now()`, presentationID, ids)
	return err
}

// assetsInContent finds the images one slide places, both in template regions
// and among the objects drawn over them.
func assetsInContent(content []byte) []string {
	if len(content) == 0 {
		return nil
	}
	var shape struct {
		Images map[string]struct {
			AssetID string `json:"assetId"`
		} `json:"images"`
		Elements []struct {
			AssetID string `json:"assetId"`
		} `json:"elements"`
	}
	if err := json.Unmarshal(content, &shape); err != nil {
		return nil
	}
	var ids []string
	for _, image := range shape.Images {
		if id := strings.TrimSpace(image.AssetID); id != "" {
			ids = append(ids, id)
		}
	}
	for _, element := range shape.Elements {
		if id := strings.TrimSpace(element.AssetID); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}
