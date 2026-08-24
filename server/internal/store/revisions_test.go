package store

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/hkjang/ptium/server/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Version history is ordered by the deck's own version, not by the clock.
//
// A clock can step backwards — a virtualised host correcting its time is
// enough, and one workspace of 2,795 decks holds seven checkpoints stamped
// earlier than the checkpoint before them. Ordering by that stamp puts an old
// checkpoint at the front of the history, and "restore the newest" then gives
// back a deck from several changes ago.
//
// Needs a database: set PTIUM_TEST_DSN to run it.
func TestHistoryIsOrderedByVersionNotByTheClock(t *testing.T) {
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

	owner, err := store.UpsertUser(ctx, "history-order", "history-order@ptium.test", "history", []string{}, false)
	if err != nil {
		t.Fatalf("owner: %v", err)
	}
	deck, err := store.CreatePresentation(ctx, owner.ID, PresentationInput{
		Title: "체크포인트 순서", Prompt: "점검", Theme: "corporate", Language: "ko",
		Audience: "임원", Tone: "간결", SlideCount: 1,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() { _, _ = pool.Exec(ctx, `DELETE FROM presentations WHERE id=$1`, deck.ID) }()

	slides := func(title string) []model.Slide {
		return []model.Slide{{Position: 1, Title: title, Content: json.RawMessage(`{}`), Layout: "content"}}
	}
	for _, title := range []string{"첫 판", "둘째 판", "셋째 판"} {
		if err := store.ReplaceSlidesFromSource(ctx, deck.ID, owner.ID, "# "+title+"\n@content\n",
			json.RawMessage(`[]`), slides(title), nil); err != nil {
			t.Fatalf("write %s: %v", title, err)
		}
	}
	// The clock steps backwards between two changes, the way a host correcting
	// its time does.
	if _, err := pool.Exec(ctx, `UPDATE presentation_revisions SET created_at = created_at - interval '2 seconds'
		WHERE presentation_id=$1 AND version=(SELECT max(version) FROM presentation_revisions WHERE presentation_id=$1)`,
		deck.ID); err != nil {
		t.Fatalf("step the clock: %v", err)
	}
	listed, total, err := store.ListPresentationRevisions(ctx, deck.ID, owner.ID, 25, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != len(listed) || len(listed) < 3 {
		t.Fatalf("expected every checkpoint, got %d of %d", len(listed), total)
	}
	for index := 1; index < len(listed); index++ {
		if listed[index-1].Version <= listed[index].Version {
			t.Fatalf("the history is not newest first: %d then %d",
				listed[index-1].Version, listed[index].Version)
		}
	}
	// And the newest one holds what the last change replaced.
	restored, err := store.PresentationRevisionSlides(ctx, deck.ID, listed[0].ID, owner.ID)
	if err != nil {
		t.Fatalf("read the newest checkpoint: %v", err)
	}
	if len(restored) != 1 || restored[0].Title != "둘째 판" {
		t.Fatalf("the newest checkpoint does not hold what the last change replaced: %#v", restored)
	}
}
