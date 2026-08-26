package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/hkjang/ptium/server/internal/model"
)

// A share is how a deck gets shown to someone who does not have an account
// here: a link that opens it read-only.
//
// The token is kept as a digest. A row of this table cannot hand anyone a
// working link — losing the database does not lose the decks — and the link
// itself exists only in the moment it is made, in the answer to the request
// that made it.
const shareTokenBytes = 24

// shareColumns is what a share row carries back, minus the digest, which never
// leaves the database.
const shareColumns = `id,presentation_id,label,expires_at,revoked_at,last_seen_at,views,created_at`

// CreateShare opens a deck to whoever holds the link it returns.
func (s *Store) CreateShare(ctx context.Context, presentationID, ownerID, label string, expires *time.Time) (model.Share, string, error) {
	if _, err := s.GetPresentation(ctx, presentationID, ownerID, false); err != nil {
		return model.Share{}, "", err
	}
	raw := make([]byte, shareTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return model.Share{}, "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	row := s.Pool.QueryRow(ctx,
		`INSERT INTO presentation_shares(presentation_id,owner_id,token_digest,label,expires_at)
		 VALUES($1,$2,$3,$4,$5) RETURNING `+shareColumns,
		presentationID, ownerID, shareDigest(token), strings.TrimSpace(label), expires)
	share, err := scanShare(row)
	if err != nil {
		return model.Share{}, "", err
	}
	return share, token, nil
}

// ListShares is every link made for one deck, so the owner can see what is open
// and close it.
func (s *Store) ListShares(ctx context.Context, presentationID, ownerID string) ([]model.Share, error) {
	if _, err := s.GetPresentation(ctx, presentationID, ownerID, false); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT `+shareColumns+` FROM presentation_shares
		WHERE presentation_id=$1 AND owner_id=$2 ORDER BY created_at DESC LIMIT 100`,
		presentationID, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	shares := make([]model.Share, 0, 8)
	for rows.Next() {
		share, err := scanShare(rows)
		if err != nil {
			return nil, err
		}
		shares = append(shares, share)
	}
	return shares, rows.Err()
}

// RevokeShare closes a link. The deck is untouched; the link stops working.
func (s *Store) RevokeShare(ctx context.Context, presentationID, ownerID, shareID string) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE presentation_shares SET revoked_at=now()
		WHERE id=$1 AND presentation_id=$2 AND owner_id=$3 AND revoked_at IS NULL`,
		shareID, presentationID, ownerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ErrShareClosed says the link was real and is not open any more, which is a
// different thing from a link that never existed and is worth saying
// differently.
var ErrShareClosed = errors.New("this link is no longer open")

// PresentationByShare is the deck behind a link, or an error saying why not.
// Reading through a link counts as a view.
func (s *Store) PresentationByShare(ctx context.Context, token string) (model.Presentation, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return model.Presentation{}, ErrNotFound
	}
	var presentationID, ownerID string
	var revoked, expires *time.Time
	err := s.Pool.QueryRow(ctx, `SELECT presentation_id,owner_id,revoked_at,expires_at
		FROM presentation_shares WHERE token_digest=$1`, shareDigest(token)).
		Scan(&presentationID, &ownerID, &revoked, &expires)
	if err != nil {
		return model.Presentation{}, ErrNotFound
	}
	if revoked != nil || (expires != nil && expires.Before(time.Now())) {
		return model.Presentation{}, ErrShareClosed
	}
	presentation, err := s.GetPresentation(ctx, presentationID, ownerID, false)
	if err != nil {
		return model.Presentation{}, err
	}
	_, _ = s.Pool.Exec(ctx, `UPDATE presentation_shares SET views=views+1,last_seen_at=now()
		WHERE token_digest=$1`, shareDigest(token))
	return presentation, nil
}

func shareDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

type shareRow interface {
	Scan(dest ...any) error
}

func scanShare(row shareRow) (model.Share, error) {
	var share model.Share
	var expires, revoked, seen *time.Time
	if err := row.Scan(&share.ID, &share.PresentationID, &share.Label, &expires, &revoked, &seen,
		&share.Views, &share.CreatedAt); err != nil {
		return model.Share{}, err
	}
	share.ExpiresAt, share.RevokedAt, share.LastSeenAt = expires, revoked, seen
	return share, nil
}

// OpenShare is one link an operator can see across the whole deployment, with
// the deck and the person it belongs to.
//
// A link is the one thing in this product that reaches somebody with no account
// here: at a closed site that is the question an operator is asked. Until now
// only the deck's owner could see their own links, so nobody could answer what
// is open, made by whom, and still being read.
type OpenShare struct {
	model.Share
	DeckTitle  string `json:"deckTitle"`
	OwnerEmail string `json:"ownerEmail,omitempty"`
	// OwnerName is what the account is called. An account can have no address —
	// one signed in through a proxy, or the deployment's own developer identity
	// — and a list of links whose owner column is blank names nobody.
	OwnerName string `json:"ownerName,omitempty"`
	OwnerID   string `json:"ownerId"`
	// State is what the link does now: open, expired or revoked.
	State string `json:"state"`
}

// SharesFilter narrows the list to the question being asked.
type SharesFilter struct {
	// State is open, expired or revoked. Empty means all of them.
	State string
	// Search matches the label, the deck's title or the owner's address.
	Search string
}

// ListAllShares reads every link in the deployment, newest first.
func (s *Store) ListAllShares(ctx context.Context, filter SharesFilter, limit, offset int) ([]OpenShare, int, error) {
	limit, offset = clampPage(limit, offset)
	state := strings.ToLower(strings.TrimSpace(filter.State))
	search := strings.TrimSpace(filter.Search)
	where := `WHERE p.deleted_at IS NULL
		AND ($1='' OR CASE WHEN s.revoked_at IS NOT NULL THEN 'revoked'
			WHEN s.expires_at IS NOT NULL AND s.expires_at <= now() THEN 'expired' ELSE 'open' END = $1)
		AND ($2='' OR s.label ILIKE $3 OR p.title ILIKE $3 OR COALESCE(u.email,'') ILIKE $3
			OR COALESCE(u.name,'') ILIKE $3)`
	pattern := likePattern(search)
	var total int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM presentation_shares s
		JOIN presentations p ON p.id = s.presentation_id
		LEFT JOIN users u ON u.id = s.owner_id `+where, state, search, pattern).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT s.id::text, s.presentation_id::text, COALESCE(s.label,''),
			s.expires_at, s.revoked_at, s.last_seen_at, s.views, s.created_at,
			p.title, s.owner_id::text, COALESCE(u.email,''), COALESCE(u.name,''),
			CASE WHEN s.revoked_at IS NOT NULL THEN 'revoked'
				WHEN s.expires_at IS NOT NULL AND s.expires_at <= now() THEN 'expired' ELSE 'open' END
		FROM presentation_shares s
		JOIN presentations p ON p.id = s.presentation_id
		LEFT JOIN users u ON u.id = s.owner_id `+where+`
		ORDER BY s.created_at DESC LIMIT $4 OFFSET $5`, state, search, pattern, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	open := make([]OpenShare, 0, limit)
	for rows.Next() {
		var one OpenShare
		if err := rows.Scan(&one.ID, &one.PresentationID, &one.Label, &one.ExpiresAt, &one.RevokedAt,
			&one.LastSeenAt, &one.Views, &one.CreatedAt, &one.DeckTitle, &one.OwnerID, &one.OwnerEmail,
			&one.OwnerName, &one.State); err != nil {
			return nil, 0, err
		}
		open = append(open, one)
	}
	return open, total, rows.Err()
}

// CloseShare closes a link whoever made it, which is what an operator asked to
// shut something down needs. It answers whether it closed anything: a link
// somebody revoked a moment earlier is not closed again, and saying nothing
// there would tell an operator they did something they did not.
func (s *Store) CloseShare(ctx context.Context, shareID string) (OpenShare, bool, error) {
	tag, err := s.Pool.Exec(ctx, `UPDATE presentation_shares SET revoked_at=now()
		WHERE id=$1 AND revoked_at IS NULL`, shareID)
	if err != nil {
		return OpenShare{}, false, err
	}
	var one OpenShare
	err = s.Pool.QueryRow(ctx, `SELECT s.id::text, s.presentation_id::text, COALESCE(s.label,''),
			s.expires_at, s.revoked_at, s.last_seen_at, s.views, s.created_at,
			p.title, s.owner_id::text, COALESCE(u.email,''), COALESCE(u.name,''), 'revoked'
		FROM presentation_shares s JOIN presentations p ON p.id = s.presentation_id
		LEFT JOIN users u ON u.id = s.owner_id WHERE s.id=$1`, shareID).Scan(
		&one.ID, &one.PresentationID, &one.Label, &one.ExpiresAt, &one.RevokedAt, &one.LastSeenAt,
		&one.Views, &one.CreatedAt, &one.DeckTitle, &one.OwnerID, &one.OwnerEmail, &one.OwnerName, &one.State)
	if err != nil {
		return OpenShare{}, false, mapNotFound(err)
	}
	return one, tag.RowsAffected() == 1, nil
}
