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

	// What one kind of action did, and what one person did.
	if _, total, err = store.ListAuditTrail(ctx, AuditFilter{Action: "presentation", Search: run}, 50, 0); err != nil || total != 2 {
		t.Errorf("filtering by an action prefix found %d, want 2 (err %v)", total, err)
	}
	if _, total, err = store.ListAuditTrail(ctx, AuditFilter{Actor: run + "@ptium.test"}, 50, 0); err != nil || total != 3 {
		t.Errorf("filtering by an actor found %d, want 3 (err %v)", total, err)
	}
	if _, total, err = store.ListAuditTrail(ctx, AuditFilter{Target: "settings", Search: run}, 50, 0); err != nil || total != 1 {
		t.Errorf("filtering by a target kind found %d, want 1 (err %v)", total, err)
	}
	// And what was recorded alongside it, which is where the detail lives.
	if _, total, err = store.ListAuditTrail(ctx, AuditFilter{Search: run}, 50, 0); err != nil || total != 3 {
		t.Errorf("searching what was recorded found %d, want 3 (err %v)", total, err)
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
