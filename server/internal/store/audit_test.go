package store

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Thirty-five places in this server write an audit record and, until the trail
// had a reader, nothing read one: an operator asking "who turned the provider
// on" or "who deleted that deck" had a table they could only reach with psql.
//
// Needs a database: set PTIUM_TEST_DSN to run it.
func TestTheAuditTrailAnswersWhoDidWhat(t *testing.T) {
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

	run := fmt.Sprintf("audit-%d", time.Now().UnixNano())
	actor, err := store.UpsertUser(ctx, "dev:"+run+"@ptium.test", run+"@ptium.test", run, []string{"user"}, false)
	if err != nil {
		t.Fatalf("actor: %v", err)
	}
	deck := run + "-deck"
	store.Audit(ctx, &actor.ID, "presentation.create", "presentation", deck, map[string]any{"run": run})
	store.Audit(ctx, &actor.ID, "presentation.trash", "presentation", deck, map[string]any{"run": run})
	store.Audit(ctx, &actor.ID, "settings.update", "settings", "ai.provider", map[string]any{"run": run})

	// What happened to one thing.
	entries, total, err := store.ListAuditTrail(ctx, AuditFilter{TargetID: deck}, 50, 0)
	if err != nil {
		t.Fatalf("read the trail: %v", err)
	}
	if total != 2 || len(entries) != 2 {
		t.Fatalf("a deck with two entries reads as %d (%d rows)", total, len(entries))
	}
	// Newest first: an operator opens the trail to see what just happened.
	if entries[0].Action != "presentation.trash" {
		t.Errorf("the trail reads %q first, want the most recent", entries[0].Action)
	}
	// The actor is a person, not a uuid.
	if entries[0].ActorEmail != run+"@ptium.test" {
		t.Errorf("the entry names its actor as %q", entries[0].ActorEmail)
	}

	// A family of actions, and one of them. The name of a family stops at the
	// dot: asking for presentation.create must not also answer with
	// presentation.create_and_generate.
	store.Audit(ctx, &actor.ID, "presentation.create_and_generate", "presentation", deck, map[string]any{"run": run})
	if _, total, err = store.ListAuditTrail(ctx, AuditFilter{Action: "presentation", Search: run}, 50, 0); err != nil || total != 3 {
		t.Errorf("filtering by a family found %d, want 3 (err %v)", total, err)
	}
	if _, total, err = store.ListAuditTrail(ctx, AuditFilter{Action: "presentation.create", Search: run}, 50, 0); err != nil || total != 1 {
		t.Errorf("filtering by one action found %d, want 1 — the family's other members came too (err %v)", total, err)
	}
	if _, total, err = store.ListAuditTrail(ctx, AuditFilter{Actor: run + "@ptium.test"}, 50, 0); err != nil || total != 4 {
		t.Errorf("filtering by an actor found %d, want 4 (err %v)", total, err)
	}
	if _, total, err = store.ListAuditTrail(ctx, AuditFilter{Target: "settings", Search: run}, 50, 0); err != nil || total != 1 {
		t.Errorf("filtering by a target kind found %d, want 1 (err %v)", total, err)
	}
	// And what was recorded alongside it, which is where the detail lives.
	if _, total, err = store.ListAuditTrail(ctx, AuditFilter{Search: run}, 50, 0); err != nil || total != 4 {
		t.Errorf("searching what was recorded found %d, want 4 (err %v)", total, err)
	}
	// Nothing from before the window asked for.
	if _, total, err = store.ListAuditTrail(ctx, AuditFilter{Search: run, Since: time.Now().Add(time.Hour)}, 50, 0); err != nil || total != 0 {
		t.Errorf("a window in the future found %d entries (err %v)", total, err)
	}
	// The actions a filter can offer are the ones this deployment writes.
	actions, err := store.AuditActions(ctx, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("read the actions: %v", err)
	}
	seen := map[string]int{}
	for _, action := range actions {
		seen[action.Action] = action.Count
	}
	if seen["presentation.trash"] < 1 || seen["settings.update"] < 1 {
		t.Errorf("the actions do not include what was just written: %v", seen)
	}

	if _, err := pool.Exec(ctx, `DELETE FROM audit_logs WHERE actor_id=$1`, actor.ID); err != nil {
		t.Fatalf("clean up: %v", err)
	}
}

// An operator who can see that a deck has been waiting twenty minutes should be
// able to do something about it. Until the queue had a reader, a deck belonged
// to its owner and an administrator could not see one.
//
// Needs a database: set PTIUM_TEST_DSN to run it.
func TestTheQueueIsVisibleAndStoppable(t *testing.T) {
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

	run := fmt.Sprintf("queue-%d", time.Now().UnixNano())
	owner, err := store.UpsertUser(ctx, "dev:"+run+"@ptium.test", run+"@ptium.test", run, []string{"user"}, false)
	if err != nil {
		t.Fatalf("owner: %v", err)
	}
	deck, err := store.CreatePresentation(ctx, owner.ID, PresentationInput{
		Title: run, Prompt: "점검", Language: "ko", SlideCount: 6, Theme: "slate-classic"})
	if err != nil {
		t.Fatalf("deck: %v", err)
	}
	if _, err := store.QueueGeneration(ctx, deck.ID, owner.ID, false, 50); err != nil {
		t.Fatalf("queue: %v", err)
	}

	queue, err := store.GenerationQueue(ctx, 24, 100)
	if err != nil {
		t.Fatalf("read the queue: %v", err)
	}
	var mine *QueuedDeck
	for index := range queue {
		if queue[index].ID == deck.ID {
			mine = &queue[index]
		}
	}
	if mine == nil {
		t.Fatalf("a queued deck is not in the queue an operator reads")
	}
	// Whose it is, said as an address rather than a uuid.
	if mine.OwnerEmail != run+"@ptium.test" {
		t.Errorf("the queue names the owner %q", mine.OwnerEmail)
	}

	// The lease a worker holds while it writes this deck. A late report is only
	// the holder's to make, so the test has to speak as the worker that claimed
	// it rather than as nobody.
	var workerLease string
	if err := pool.QueryRow(ctx, `UPDATE presentations SET generation_lease=gen_random_uuid() WHERE id=$1
		RETURNING generation_lease::text`, deck.ID).Scan(&workerLease); err != nil {
		t.Fatalf("hand it a lease: %v", err)
	}

	// Stopping one gives a reason, and the reason is what its author reads.
	const reason = "예산 승인 전까지 중단합니다"
	didStop, err := store.StopGeneration(ctx, deck.ID, reason)
	if err != nil || !didStop {
		t.Fatalf("stop it: stopped=%v err=%v", didStop, err)
	}
	// And a deck that is no longer in the queue is not stopped, and says so
	// rather than answering as though it were.
	if again, err := store.StopGeneration(ctx, deck.ID, "두 번째 중단"); err != nil || again {
		t.Errorf("stopping a deck that already finished answered stopped=%v (err %v)", again, err)
	}
	stopped, err := store.GetPresentation(ctx, deck.ID, owner.ID, false)
	if err != nil {
		t.Fatalf("read it back: %v", err)
	}
	if stopped.Status != "failed" || stopped.ErrorMessage != reason {
		t.Fatalf("a stopped deck reads as %q / %q", stopped.Status, stopped.ErrorMessage)
	}
	// A worker finishing a moment later must not overwrite that reason: the deck
	// is no longer being written, and what the operator said stands.
	if err := store.FailGeneration(ctx, deck.ID, workerLease, "생성에 실패했습니다. 다시 시도해 주세요."); err != nil {
		t.Fatalf("the late failure errored: %v", err)
	}
	again, err := store.GetPresentation(ctx, deck.ID, owner.ID, false)
	if err != nil {
		t.Fatalf("read it back: %v", err)
	}
	if again.ErrorMessage != reason {
		t.Errorf("a worker finishing later overwrote the operator's reason with %q", again.ErrorMessage)
	}

	// And the other way the two verbs are not the same: an operator pushes a
	// stuck deck back into the queue, and a worker still holding the previous
	// attempt reports its failure. The deck waiting in the queue is not that
	// worker's to fail — it killed the fresh attempt before anybody picked it up.
	if _, err := store.QueueGeneration(ctx, deck.ID, owner.ID, false, 50); err != nil {
		t.Fatalf("push it back: %v", err)
	}
	if err := store.FailGeneration(ctx, deck.ID, workerLease, "생성에 실패했습니다. 다시 시도해 주세요."); err != nil {
		t.Fatalf("the straggler errored: %v", err)
	}
	waiting, err := store.GetPresentation(ctx, deck.ID, owner.ID, false)
	if err != nil {
		t.Fatalf("read it back: %v", err)
	}
	if waiting.Status != "queued" {
		t.Errorf("a deck waiting in the queue reads as %q after a straggler reported: %q",
			waiting.Status, waiting.ErrorMessage)
	}

	if _, err := pool.Exec(ctx, `DELETE FROM presentations WHERE id=$1`, deck.ID); err != nil {
		t.Fatalf("clean up: %v", err)
	}
}
