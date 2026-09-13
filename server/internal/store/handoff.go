package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// A deck on its way to another service (HANDOFF-STANDARD).
//
// The claim is the whole credential, so the row keeps its digest and never the
// claim; the file is built when the claim is issued, under the permissions of
// the person who asked, because the peer that comes for it brings no identity
// of its own. Redeeming is a delete that returns the row: the second request
// for the same claim finds nothing, and so does one after the five minutes.

// HandoffClaim is a built file waiting for the one peer that holds its claim.
type HandoffClaim struct {
	PresentationID string
	OwnerID        string
	Format         string
	Filename       string
	ContentType    string
	Body           []byte
	ExpiresAt      time.Time
}

// IssueHandoffClaim stores a built file under the claim's digest. Claims whose
// time has passed are swept on the way, so the table never holds more than a
// few minutes of traffic.
func (s *Store) IssueHandoffClaim(ctx context.Context, claim string, input HandoffClaim) error {
	_, _ = s.Pool.Exec(ctx, `DELETE FROM handoff_claims WHERE expires_at < now()`)
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO handoff_claims(claim_digest,presentation_id,owner_id,format,filename,content_type,body,expires_at)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
		shareDigest(claim), input.PresentationID, input.OwnerID, input.Format, input.Filename,
		input.ContentType, input.Body, input.ExpiresAt)
	return err
}

// RedeemHandoffClaim hands the file over once. A claim that was already
// redeemed, has expired, or never existed is the same ErrNotFound: the answer
// does not say which.
func (s *Store) RedeemHandoffClaim(ctx context.Context, claim string) (HandoffClaim, error) {
	var redeemed HandoffClaim
	err := s.Pool.QueryRow(ctx,
		`DELETE FROM handoff_claims WHERE claim_digest=$1 AND expires_at > now()
		 RETURNING presentation_id,owner_id,format,filename,content_type,body,expires_at`,
		shareDigest(claim)).Scan(&redeemed.PresentationID, &redeemed.OwnerID, &redeemed.Format,
		&redeemed.Filename, &redeemed.ContentType, &redeemed.Body, &redeemed.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return HandoffClaim{}, ErrNotFound
	}
	return redeemed, err
}
