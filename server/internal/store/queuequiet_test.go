package store

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// The queue an operator reads has to separate a deck that is taking a while
// from one nobody is writing.
//
// It used to call anything older than fifteen minutes stuck. That is a claim
// about how long generation takes, and this product leaves a slow generation
// alone however long it runs — so a healthy deck on a self-hosted model would
// have been reported as stuck to an operator who could have stopped it.
//
// Needs a database: set PTIUM_TEST_DSN to run it.
func TestTheQueueSaysHowLongADeckHasBeenSilent(t *testing.T) {
	dsn := os.Getenv("PTIUM_TEST_DSN")
	if dsn == "" {
		t.Skip("set PTIUM_TEST_DSN to run the database-backed store tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()
	store := New(pool)
	owner, err := store.UpsertUser(ctx, "queue-quiet", "queue-quiet@ptium.test", "quiet", []string{}, false)
	if err != nil {
		t.Fatalf("owner: %v", err)
	}
	deck, err := store.CreatePresentation(ctx, owner.ID, PresentationInput{
		Title: "조용한 덱 점검", Prompt: "브리프", Theme: "slate-classic",
		Language: "ko", Audience: "general", Tone: "professional", SlideCount: 5})
	if err != nil {
		t.Fatalf("deck: %v", err)
	}
	defer func() { _, _ = pool.Exec(ctx, `DELETE FROM presentations WHERE id=$1`, deck.ID) }()

	mine := func() QueuedDeck {
		queue, err := store.GenerationQueue(ctx, 0, 200)
		if err != nil {
			t.Fatalf("read the queue: %v", err)
		}
		for _, row := range queue {
			if row.ID == deck.ID {
				return row
			}
		}
		t.Fatal("the deck is not in the queue an operator reads")
		return QueuedDeck{}
	}

	// Waiting to be picked up: there is nothing to be silent about yet.
	if _, err := pool.Exec(ctx, `UPDATE presentations SET status='queued',updated_at=now()-interval '20 minutes' WHERE id=$1`, deck.ID); err != nil {
		t.Fatalf("queue it: %v", err)
	}
	waiting := mine()
	if waiting.QuietFor != nil {
		t.Errorf("a deck nobody has claimed reports %d seconds of silence", *waiting.QuietFor)
	}
	if waiting.WaitingFor < 1000 {
		t.Errorf("a deck waiting twenty minutes reports %ds", waiting.WaitingFor)
	}

	// Being written for half an hour, saying so ten seconds ago: not in trouble.
	if _, err := pool.Exec(ctx, `UPDATE presentations SET status='generating',
		generation_started_at=now()-interval '30 minutes', generation_heartbeat_at=now()-interval '10 seconds',
		generation_lease=gen_random_uuid(), updated_at=now()-interval '30 minutes' WHERE id=$1`, deck.ID); err != nil {
		t.Fatalf("hand it to a worker: %v", err)
	}
	writing := mine()
	if writing.QuietFor == nil {
		t.Fatal("a deck being written does not say how long it has been silent")
	}
	if *writing.QuietFor > 60 {
		t.Errorf("a worker that spoke ten seconds ago reads as %ds of silence", *writing.QuietFor)
	}

	// And when the worker stops saying anything, the queue says how long for.
	if _, err := pool.Exec(ctx, `UPDATE presentations SET generation_heartbeat_at=now()-interval '9 minutes' WHERE id=$1`, deck.ID); err != nil {
		t.Fatalf("go quiet: %v", err)
	}
	if silent := mine(); silent.QuietFor == nil || *silent.QuietFor < 500 {
		t.Errorf("a worker silent for nine minutes reads as %v", silent.QuietFor)
	}
}
