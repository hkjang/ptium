package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
		return model.Asset{}, errors.New("an image needs a name")
	}
	if len(in.Data) == 0 {
		return model.Asset{}, errors.New("the image is empty")
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
	var asset model.Asset
	err := s.Pool.QueryRow(ctx, `INSERT INTO assets(id,owner_id,name,content_type,size_bytes,width,height,checksum,data)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (owner_id,lower(name)) DO UPDATE SET
			content_type=EXCLUDED.content_type,size_bytes=EXCLUDED.size_bytes,
			width=EXCLUDED.width,height=EXCLUDED.height,checksum=EXCLUDED.checksum,data=EXCLUDED.data
		RETURNING id::text,owner_id::text,name,content_type,size_bytes,width,height,checksum,created_at`,
		newID(), ownerID, name, contentType, len(in.Data), width, height,
		hex.EncodeToString(digest[:]), in.Data).Scan(&asset.ID, &asset.OwnerID, &asset.Name,
		&asset.ContentType, &asset.SizeBytes, &asset.Width, &asset.Height, &asset.Checksum, &asset.CreatedAt)
	return asset, err
}

// ListAssets returns a user's images without their bytes.
func (s *Store) ListAssets(ctx context.Context, ownerID string, limit, offset int) ([]model.Asset, int, error) {
	limit, offset = clampPage(limit, offset)
	var total int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM assets WHERE owner_id=$1`, ownerID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT id::text,owner_id::text,name,content_type,size_bytes,width,height,checksum,created_at
		FROM assets WHERE owner_id=$1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`, ownerID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := make([]model.Asset, 0)
	for rows.Next() {
		var asset model.Asset
		if err := rows.Scan(&asset.ID, &asset.OwnerID, &asset.Name, &asset.ContentType,
			&asset.SizeBytes, &asset.Width, &asset.Height, &asset.Checksum, &asset.CreatedAt); err != nil {
			return nil, 0, err
		}
		result = append(result, asset)
	}
	return result, total, rows.Err()
}

// AssetData returns an image's bytes.
func (s *Store) AssetData(ctx context.Context, id, ownerID string) ([]byte, model.Asset, error) {
	var asset model.Asset
	var data []byte
	err := s.Pool.QueryRow(ctx, `SELECT id::text,owner_id::text,name,content_type,size_bytes,width,height,checksum,created_at,data
		FROM assets WHERE id=$1 AND owner_id=$2`, id, ownerID).Scan(&asset.ID, &asset.OwnerID, &asset.Name,
		&asset.ContentType, &asset.SizeBytes, &asset.Width, &asset.Height, &asset.Checksum, &asset.CreatedAt, &data)
	if err != nil {
		return nil, model.Asset{}, mapNotFound(err)
	}
	return data, asset, nil
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
