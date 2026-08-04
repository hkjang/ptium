package store

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/hkjang/ptium/server/internal/pptx"
)

// ErrGridInvalid reports a definition Ptium cannot draw from.
var ErrGridInvalid = errors.New("invalid grid definition")

// gridNamePattern keeps a definition's name usable in deck source, where it is
// one token followed by an optional caption.
var gridNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,39}$`)

// SaveGrid stores a grid definition for a user, replacing one of the same name.
func (s *Store) SaveGrid(ctx context.Context, ownerID string, spec pptx.GridSpec) (pptx.GridSpec, error) {
	spec.Name = strings.ToLower(strings.TrimSpace(spec.Name))
	if !gridNamePattern.MatchString(spec.Name) {
		return pptx.GridSpec{}, errors.New("a grid name is 2-40 characters of lowercase letters, digits, hyphen or underscore")
	}
	if len(spec.Values) == 0 && len(spec.Columns) == 0 {
		return pptx.GridSpec{}, errors.New("a grid definition needs columns, values, or both")
	}
	if len(spec.Order) > 24 {
		return pptx.GridSpec{}, errors.New("a grid definition allows up to 24 ordered values")
	}
	if len(spec.Values) > 24 || len(spec.Columns) > 8 {
		return pptx.GridSpec{}, errors.New("a grid definition allows up to 8 columns and 24 values")
	}
	for value, entry := range spec.Values {
		if strings.TrimSpace(value) == "" {
			return pptx.GridSpec{}, errors.New("a grid value cannot be blank")
		}
		if !validGridRole(entry.Role) {
			return pptx.GridSpec{}, errors.New("a grid value's role must be accent1-accent6, positive, negative, muted or ink")
		}
	}
	encoded, err := json.Marshal(spec)
	if err != nil {
		return pptx.GridSpec{}, err
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO grids(id,owner_id,name,spec) VALUES($1,$2,$3,$4)
		ON CONFLICT (owner_id,lower(name)) DO UPDATE SET spec=EXCLUDED.spec,updated_at=now()`,
		newID(), ownerID, spec.Name, encoded); err != nil {
		return pptx.GridSpec{}, err
	}
	return spec, nil
}

// validGridRole reports whether a colour role is one a template can resolve. A
// definition never names a colour, so this is the whole vocabulary.
func validGridRole(role string) bool {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "", "ink", "muted", "positive", "negative",
		"accent1", "accent2", "accent3", "accent4", "accent5", "accent6":
		return true
	}
	return false
}

// ListGrids returns a user's definitions together with the shipped ones, which
// are what a customer copies to make their own.
func (s *Store) ListGrids(ctx context.Context, ownerID string) ([]pptx.GridSpec, error) {
	rows, err := s.Pool.Query(ctx, `SELECT spec FROM grids WHERE owner_id=$1 ORDER BY lower(name)`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	owned := make([]pptx.GridSpec, 0)
	names := map[string]bool{}
	for rows.Next() {
		var encoded []byte
		if err := rows.Scan(&encoded); err != nil {
			return nil, err
		}
		var spec pptx.GridSpec
		if json.Unmarshal(encoded, &spec) != nil {
			continue
		}
		owned = append(owned, spec)
		names[spec.Name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// A definition of the same name shadows the shipped one, which is how a
	// customer replaces "raci" with their own.
	for _, spec := range pptx.BuiltinGrids() {
		if !names[spec.Name] {
			owned = append(owned, spec)
		}
	}
	return owned, nil
}

// ResolveGrid finds the definition deck source names.
func (s *Store) ResolveGrid(ctx context.Context, ownerID, name string) (pptx.GridSpec, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return pptx.GridSpec{}, false
	}
	var encoded []byte
	err := s.Pool.QueryRow(ctx, `SELECT spec FROM grids WHERE owner_id=$1 AND lower(name)=$2`, ownerID, name).Scan(&encoded)
	if err == nil {
		var spec pptx.GridSpec
		if json.Unmarshal(encoded, &spec) == nil {
			return spec, true
		}
	}
	return pptx.LookupBuiltinGrid(name)
}

// DeleteGrid removes a definition. A shipped definition cannot be deleted, only
// shadowed, so removing an override restores it.
func (s *Store) DeleteGrid(ctx context.Context, ownerID, name string) error {
	result, err := s.Pool.Exec(ctx, `DELETE FROM grids WHERE owner_id=$1 AND lower(name)=$2`,
		ownerID, strings.ToLower(strings.TrimSpace(name)))
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
