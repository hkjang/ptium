package store

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// The queue screen counted the rows it had been handed, and it is handed a
// hundred: a site with four hundred decks waiting read "100 대기 · 작성 중" on
// the very screen an operator opens to see how far behind it is, while the
// overview beside it counted them all and said four hundred.
//
// Needs a database: set PTIUM_TEST_DSN to run it.
func TestTheQueueSaysHowMuchItHoldsNotHowMuchFits(t *testing.T) {
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

	owner, err := store.UpsertUser(ctx, "queue-totals", "queue-totals@ptium.test", "queue", []string{}, false)
	if err != nil {
		t.Fatalf("owner: %v", err)
	}
	// More than the list can carry, so the difference is the whole point.
	const seeded = 130
	if _, err := pool.Exec(ctx, `INSERT INTO presentations (id, owner_id, title, prompt, status, created_at, updated_at)
		SELECT gen_random_uuid(), $1, '큐 총계 시험 ' || g, '시험', 'queued',
			now() - (g || ' minutes')::interval, now() - (g || ' minutes')::interval
		FROM generate_series(1, $2) g`, owner.ID, seeded); err != nil {
		t.Fatalf("seed: %v", err)
	}
	defer func() { _, _ = pool.Exec(ctx, `DELETE FROM presentations WHERE owner_id=$1`, owner.ID) }()

	list, err := store.GenerationQueue(ctx, 24, 100)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if len(list) != 100 {
		t.Fatalf("the list carried %d rows, want the 100 it was asked for", len(list))
	}
	totals, err := store.GenerationQueueTotals(ctx, 24)
	if err != nil {
		t.Fatalf("totals: %v", err)
	}
	if totals.Waiting < seeded {
		t.Fatalf("%d decks are waiting and the queue says %d", seeded, totals.Waiting)
	}
	if totals.Waiting <= len(list) {
		t.Fatalf("the queue says it holds %d and its list carries %d: the cap is not being reported past",
			totals.Waiting, len(list))
	}
}
