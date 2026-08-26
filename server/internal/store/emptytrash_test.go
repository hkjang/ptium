package store

import (
	"context"
	"os"
	"testing"

	"github.com/hkjang/ptium/server/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Emptying the recycle bin deletes what is in the recycle bin, and nothing
// else. A deck somebody is working on and a deck belonging to somebody else are
// both out of its reach — the first because it was never deleted, the second
// because it is not theirs.
//
// This one is worth deciding rather than trusting: a wrong condition here does
// not report an error, it removes work nobody can get back.
//
// Needs a database: set PTIUM_TEST_DSN to run it.
func TestEmptyingTheTrashTakesOnlyTheTrash(t *testing.T) {
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

	mine, err := store.UpsertUser(ctx, "empty-trash-mine", "empty-trash-mine@ptium.test", "mine", []string{}, false)
	if err != nil {
		t.Fatalf("owner: %v", err)
	}
	theirs, err := store.UpsertUser(ctx, "empty-trash-theirs", "empty-trash-theirs@ptium.test", "theirs", []string{}, false)
	if err != nil {
		t.Fatalf("other owner: %v", err)
	}
	made := func(owner, title string) model.Presentation {
		deck, err := store.CreatePresentation(ctx, owner, PresentationInput{
			Title: title, Prompt: "점검", Theme: "corporate", Language: "ko", SlideCount: 1,
		})
		if err != nil {
			t.Fatalf("create %s: %v", title, err)
		}
		return deck
	}
	live := made(mine.ID, "살아 있어야 하는 덱")
	doomed := made(mine.ID, "지워질 덱")
	otherLive := made(theirs.ID, "남의 살아 있는 덱")
	otherDoomed := made(theirs.ID, "남의 지워진 덱")
	defer func() {
		for _, deck := range []model.Presentation{live, doomed, otherLive, otherDoomed} {
			_, _ = pool.Exec(ctx, `DELETE FROM presentations WHERE id=$1`, deck.ID)
		}
	}()
	for _, deck := range []model.Presentation{doomed, otherDoomed} {
		if err := store.DeletePresentation(ctx, deck.ID, "", true); err != nil {
			t.Fatalf("delete: %v", err)
		}
	}

	deleted, err := store.EmptyTrash(ctx, mine.ID, false)
	if err != nil {
		t.Fatalf("EmptyTrash: %v", err)
	}
	if deleted != 1 {
		t.Errorf("EmptyTrash() removed %d decks, want the one in this owner's trash", deleted)
	}
	for _, kept := range []model.Presentation{live, otherLive, otherDoomed} {
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM presentations WHERE id=$1)`, kept.ID).Scan(&exists); err != nil {
			t.Fatalf("read back: %v", err)
		}
		if !exists {
			t.Errorf("%q was taken by somebody else's empty-trash", kept.Title)
		}
	}
	var gone bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM presentations WHERE id=$1)`, doomed.ID).Scan(&gone); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if gone {
		t.Error("the deck in the trash is still there")
	}
}
