package generation

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

// A worker that loses its deck mid-flight must not report a fault.
//
// The lease exists so a second worker cannot write over a deck the first one is
// writing, and so a deck somebody deleted or stopped is not finished anyway.
// When it does its job the losing attempt gets its completion refused — and
// that refusal was being recorded in the error centre at error severity. It
// became the largest group there by a wide margin: a hundred and fifty
// "presentation generation state changed" entries, every one of them against a
// deck that had finished perfectly well, sitting on top of the faults an
// operator actually has to act on.
//
// Needs a database: set PTIUM_TEST_DSN to run it.
func TestLosingTheRaceForADeckIsNotReportedAsAFault(t *testing.T) {
	dsn := os.Getenv("PTIUM_TEST_DSN")
	if dsn == "" {
		t.Skip("set PTIUM_TEST_DSN to run the database-backed tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()
	data := store.New(pool)
	run := fmt.Sprintf("lostrace-%d", time.Now().UnixNano())
	owner, err := data.UpsertUser(ctx, "dev:"+run, run+"@ptium.test", run, []string{"user"}, false)
	if err != nil {
		t.Fatalf("owner: %v", err)
	}
	deck, err := data.CreatePresentation(ctx, owner.ID, store.PresentationInput{
		Title: "경합에서 진 덱", Prompt: "브리프", Theme: "slate-classic",
		Language: "ko", Audience: "general", Tone: "professional", SlideCount: 5})
	if err != nil {
		t.Fatalf("deck: %v", err)
	}
	defer func() { _, _ = pool.Exec(ctx, `DELETE FROM presentations WHERE id=$1`, deck.ID) }()

	// Held by this worker, the way a claim leaves it. The lease is written here
	// rather than claimed so the test does not race whatever worker the
	// deployment under test is running.
	var lease string
	if err := pool.QueryRow(ctx, `UPDATE presentations SET status='generating',generation_lease=gen_random_uuid(),
		generation_heartbeat_at=now(),updated_at=now() WHERE id=$1 RETURNING generation_lease::text`,
		deck.ID).Scan(&lease); err != nil {
		t.Fatalf("hold it: %v", err)
	}
	// Whoever else was writing it got there first, exactly as a completion,
	// a stop or a delete leaves the row.
	if _, err := pool.Exec(ctx, `UPDATE presentations SET status='completed',generation_lease=NULL WHERE id=$1`, deck.ID); err != nil {
		t.Fatalf("finish it elsewhere: %v", err)
	}

	before := incidentCount(t, data)
	worker := NewWorker(data, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), time.Minute)
	// The message is this run's own. What the worker keys off is the lease, not
	// the wording — and a test that reuses a real message folds its writes into
	// whatever group the deployment already has under that name, quietly
	// rewriting a record an operator reads.
	if err := worker.fail(ctx, model.Presentation{ID: deck.ID, OwnerID: owner.ID, Language: "ko"}, lease,
		errors.New("generation state changed under "+run)); err != nil {
		t.Fatalf("losing the race was reported to the loop as an error: %v", err)
	}
	if after := incidentCount(t, data); after != before {
		t.Errorf("a lost race recorded %d incident(s); the fence doing its job is not a fault", after-before)
	}

	// A deck this worker still holds is the other case: that failure is real and
	// has to reach the operator.
	if _, err := pool.Exec(ctx, `UPDATE presentations SET status='generating',generation_lease=$2::uuid WHERE id=$1`, deck.ID, lease); err != nil {
		t.Fatalf("hand it back: %v", err)
	}
	if err := worker.fail(ctx, model.Presentation{ID: deck.ID, OwnerID: owner.ID, Language: "ko"}, lease,
		errors.New("the model host refused "+run)); err == nil {
		t.Error("a real failure was swallowed")
	}
	found := false
	incidents, _, err := data.ListIncidents(ctx, "", 200, 0)
	if err != nil {
		t.Fatalf("list incidents: %v", err)
	}
	for _, incident := range incidents {
		if incident.Message == "the model host refused "+run {
			found = true
			_, _ = pool.Exec(ctx, `DELETE FROM server_errors WHERE id=$1`, incident.ID)
		}
	}
	if !found {
		t.Error("a failure on a deck this worker still holds never reached the error centre")
	}
}

func incidentCount(t *testing.T, data *store.Store) int {
	t.Helper()
	_, total, err := data.ListIncidents(context.Background(), "", 1, 0)
	if err != nil {
		t.Fatalf("count incidents: %v", err)
	}
	return total
}
