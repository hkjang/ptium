package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// A claim is spent by being redeemed: the second peer to bring it, or the
// first to bring it late, finds nothing — and the answer does not say which.
//
// Needs a database: set PTIUM_TEST_DSN to run it.
func TestAHandoffClaimIsHandedOverOnceAndNotAfterItsTime(t *testing.T) {
	dsn := os.Getenv("PTIUM_TEST_DSN")
	if dsn == "" {
		t.Skip("set PTIUM_TEST_DSN to run the database-backed store tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()
	store := New(pool)
	ctx := context.Background()

	run := fmt.Sprintf("handoff-%d", time.Now().UnixNano())
	owner, err := store.UpsertUser(ctx, "dev:"+run+"@ptium.test", run+"@ptium.test", run, []string{"user"}, false)
	if err != nil {
		t.Fatalf("owner: %v", err)
	}
	deck, err := store.CreatePresentation(ctx, owner.ID, PresentationInput{Title: run, Theme: "modern", Language: "ko", Audience: "general", Tone: "professional", SlideCount: 3})
	if err != nil {
		t.Fatalf("deck: %v", err)
	}
	body := []byte("PK\x03\x04 not really a package, and " + run)
	claim := "claim-" + run + "-abcdefghijklmnop"
	if err := store.IssueHandoffClaim(ctx, claim, HandoffClaim{
		PresentationID: deck.ID, OwnerID: owner.ID, Format: "pptx", Filename: run + ".pptx",
		ContentType: "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		Body:        body, ExpiresAt: time.Now().Add(5 * time.Minute),
	}); err != nil {
		t.Fatalf("issue: %v", err)
	}
	// The claim itself is nowhere in the table.
	var stored int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM handoff_claims WHERE claim_digest=$1`, claim).Scan(&stored); err != nil || stored != 0 {
		t.Fatalf("the claim is stored in the clear (%d, %v)", stored, err)
	}

	first, err := store.RedeemHandoffClaim(ctx, claim)
	if err != nil {
		t.Fatalf("first redeem: %v", err)
	}
	if first.PresentationID != deck.ID || first.Filename != run+".pptx" || string(first.Body) != string(body) {
		t.Fatalf("redeemed = %+v", first)
	}
	if _, err := store.RedeemHandoffClaim(ctx, claim); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second redeem: %v", err)
	}

	// One whose time has passed is the same nothing.
	late := "late-" + run + "-abcdefghijklmnop"
	if err := store.IssueHandoffClaim(ctx, late, HandoffClaim{
		PresentationID: deck.ID, OwnerID: owner.ID, Format: "pptx", Filename: "late.pptx",
		ContentType: "application/octet-stream", Body: body, ExpiresAt: time.Now().Add(-time.Second),
	}); err != nil {
		t.Fatalf("issue late: %v", err)
	}
	if _, err := store.RedeemHandoffClaim(ctx, late); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired redeem: %v", err)
	}
	// And issuing sweeps what has expired, so the table stays a few minutes long.
	if err := store.IssueHandoffClaim(ctx, "sweep-"+run+"-abcdefghijklmnop", HandoffClaim{
		PresentationID: deck.ID, OwnerID: owner.ID, Format: "pptx", Filename: "x.pptx",
		ContentType: "application/octet-stream", Body: body, ExpiresAt: time.Now().Add(time.Minute),
	}); err != nil {
		t.Fatalf("issue again: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM handoff_claims WHERE claim_digest=$1`, shareDigest(late)).Scan(&stored); err != nil || stored != 0 {
		t.Fatalf("the expired row was not swept (%d, %v)", stored, err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM presentations WHERE id=$1`, deck.ID); err != nil {
		t.Fatal(err)
	}
}
