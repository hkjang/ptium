package store

import (
	"context"
	"fmt"
	"strings"
)

// Favourite kinds. A workspace collects several sorts of thing and a person
// pins the ones they keep coming back to; the meaning does not change with the
// thing, so neither does the table.
const (
	FavoriteAsset    = "asset"
	FavoriteTemplate = "template"
	FavoriteSnippet  = "snippet"
)

// SetFavorite pins or unpins one thing for one person.
//
// Nothing checks that the reference exists. A favourite is a note someone made
// about their own workspace, and the row is removed with whatever it points at.
func (s *Store) SetFavorite(ctx context.Context, ownerID, kind, refID string, on bool) error {
	switch kind {
	case FavoriteAsset, FavoriteTemplate, FavoriteSnippet:
	default:
		return fmt.Errorf("unknown favourite kind %q", kind)
	}
	if strings.TrimSpace(refID) == "" {
		return ErrNotFound
	}
	if !on {
		_, err := s.Pool.Exec(ctx, `DELETE FROM favorites WHERE owner_id=$1 AND kind=$2 AND ref_id=$3`, ownerID, kind, refID)
		return err
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO favorites(owner_id,kind,ref_id) VALUES($1,$2,$3)
		ON CONFLICT (owner_id,kind,ref_id) DO NOTHING`, ownerID, kind, refID)
	return err
}
