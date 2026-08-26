package store

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// An operator stands a deck down and gives a reason its author will read. The
// worker that was already holding that deck finishes a moment later and writes
// what it found — and what it writes must not replace what the operator said,
// or the author is told "생성에 실패했습니다" for a deck a person stopped on purpose.
//
// The sweep covers this too, but only when the stop happens to land while a
// worker holds the deck; here it is decided rather than raced.
//
// Needs a database: set PTIUM_TEST_DSN to run it.
func TestAWorkerFinishingLaterKeepsTheReasonAnOperatorGave(t *testing.T) {
	dsn := os.Getenv("PTIUM_TEST_DSN")
	if dsn == "" {
		t.Skip("set PTIUM_TEST_DSN to run the database-backed store tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()
	store, ctx := New(pool), context.Background()

	owner, err := store.UpsertUser(ctx, "stop-reason", "stop-reason@ptium.test", "stop", []string{}, false)
	if err != nil {
		t.Fatalf("owner: %v", err)
	}
	deck, err := store.CreatePresentation(ctx, owner.ID, PresentationInput{
		Title: "중단 사유", Prompt: "점검", Theme: "corporate", Language: "ko", SlideCount: 1,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() { _, _ = pool.Exec(ctx, `DELETE FROM presentations WHERE id=$1`, deck.ID) }()

	// A worker is holding it, with the lease it claimed it under.
	var workerLease string
	if err := pool.QueryRow(ctx, `UPDATE presentations SET status='generating',generation_lease=gen_random_uuid()
		WHERE id=$1 RETURNING generation_lease::text`, deck.ID).Scan(&workerLease); err != nil {
		t.Fatalf("hand it to a worker: %v", err)
	}
	stopped, err := store.StopGeneration(ctx, deck.ID, "점검 중단")
	if err != nil || !stopped {
		t.Fatalf("StopGeneration() = %v, %v", stopped, err)
	}
	// The worker finishes and reports what it found.
	if err := store.FailGeneration(ctx, deck.ID, workerLease, "생성에 실패했습니다. 다시 시도해 주세요."); err != nil {
		t.Fatalf("FailGeneration: %v", err)
	}
	after, err := store.GetPresentation(ctx, deck.ID, owner.ID, false)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if after.ErrorMessage != "점검 중단" {
		t.Errorf("the author is told %q rather than why an operator stopped it", after.ErrorMessage)
	}
	if after.Status != "failed" {
		t.Errorf("a stopped deck reads as %q", after.Status)
	}
}
