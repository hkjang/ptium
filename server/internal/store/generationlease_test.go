package store

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// A deck being written is taken from its worker only when that worker has gone
// quiet — not merely because it is taking a while.
//
// The reclaim used to be ten minutes measured from the start, which is a guess
// about how long generation takes rather than a fact about whether anybody is
// still doing it. A self-hosted model answers in five minutes or more with
// thinking enabled and a deployment may ask for up to ten repair passes on top,
// so a healthy deck was handed to a second worker while the first was still
// waiting on the model — and both of them wrote it.
//
// Needs a database: set PTIUM_TEST_DSN to run it.
func TestASlowGenerationIsNotTakenFromTheWorkerWritingIt(t *testing.T) {
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
	owner, err := store.UpsertUser(ctx, "lease-slow", "lease-slow@ptium.test", "lease", []string{}, false)
	if err != nil {
		t.Fatalf("owner: %v", err)
	}
	deck, err := store.CreatePresentation(ctx, owner.ID, PresentationInput{
		Title: "느린 생성", Prompt: "브리프", Theme: "slate-classic",
		Language: "ko", Audience: "general", Tone: "professional", SlideCount: 5})
	if err != nil {
		t.Fatalf("deck: %v", err)
	}
	defer func() { _, _ = pool.Exec(ctx, `DELETE FROM presentations WHERE id=$1`, deck.ID) }()
	queue := func() {
		if _, err := pool.Exec(ctx, `UPDATE presentations SET status='queued',generation_lease=NULL,
			generation_heartbeat_at=NULL,updated_at=now() WHERE id=$1`, deck.ID); err != nil {
			t.Fatalf("queue: %v", err)
		}
	}
	mine := func() (string, bool) {
		claimed, lease, err := store.ClaimGeneration(ctx)
		if err != nil {
			return "", false
		}
		return lease, claimed.ID == deck.ID
	}

	queue()
	lease, ok := mine()
	if !ok {
		t.Fatal("the deck waiting in the queue was not claimed")
	}

	// Half an hour of work, saying so all along. Nobody else may take it.
	if _, err := pool.Exec(ctx, `UPDATE presentations SET generation_started_at=now()-interval '30 minutes' WHERE id=$1`, deck.ID); err != nil {
		t.Fatalf("age it: %v", err)
	}
	held, err := store.HeartbeatGeneration(ctx, deck.ID, lease)
	if err != nil || !held {
		t.Fatalf("the worker writing it could not say so: held=%v err=%v", held, err)
	}
	if _, taken := mine(); taken {
		t.Error("a deck whose worker is still writing it was handed to another worker")
	}

	// And when that worker stops saying anything, its deck goes back.
	if _, err := pool.Exec(ctx, `UPDATE presentations SET generation_heartbeat_at=now()-$1::interval WHERE id=$2`,
		(GenerationSilence * 2).String(), deck.ID); err != nil {
		t.Fatalf("go quiet: %v", err)
	}
	second, taken := mine()
	if !taken {
		t.Fatal("a deck whose worker has gone was left generating with nobody writing it")
	}
	if second == lease {
		t.Error("the deck was handed on under the same lease")
	}

	// The worker that lost it may neither finish it nor fail it, and knows.
	if held, err := store.HeartbeatGeneration(ctx, deck.ID, lease); err != nil || held {
		t.Errorf("a worker that lost its deck was told it still holds it: held=%v err=%v", held, err)
	}
	if err := store.FailGeneration(ctx, deck.ID, lease, "이 워커의 실패"); err != nil {
		t.Fatalf("the straggler errored: %v", err)
	}
	after, err := store.GetPresentation(ctx, deck.ID, owner.ID, false)
	if err != nil {
		t.Fatalf("read it back: %v", err)
	}
	if after.Status != "generating" || after.ErrorMessage != "" {
		t.Errorf("a straggler wrote over the attempt that holds the deck: %q / %q", after.Status, after.ErrorMessage)
	}
	// The worker that does hold it finishes it.
	if err := store.CompleteGeneration(ctx, deck.ID, second, []byte(`{}`), nil, "# 제목", nil); err != nil {
		t.Fatalf("the holder could not finish it: %v", err)
	}
	done, err := store.GetPresentation(ctx, deck.ID, owner.ID, false)
	if err != nil {
		t.Fatalf("read it back: %v", err)
	}
	if done.Status != "completed" {
		t.Errorf("the deck its holder finished reads as %q", done.Status)
	}
}

// An imported deck keeps what the import said about the file.
//
// Needs a database: set PTIUM_TEST_DSN to run it.
func TestAnImportedDeckKeepsWhatTheImportSaid(t *testing.T) {
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
	owner, err := store.UpsertUser(ctx, "import-notes", "import-notes@ptium.test", "notes", []string{}, false)
	if err != nil {
		t.Fatalf("owner: %v", err)
	}
	deck, err := store.CreatePresentation(ctx, owner.ID, PresentationInput{
		Title: "가져온 덱", Prompt: "브리프", Theme: "slate-classic",
		Language: "ko", Audience: "general", Tone: "professional", SlideCount: 5})
	if err != nil {
		t.Fatalf("deck: %v", err)
	}
	defer func() { _, _ = pool.Exec(ctx, `DELETE FROM presentations WHERE id=$1`, deck.ID) }()

	said := []string{"그림 22개를 이미지 라이브러리에 저장했습니다", "그 가운데 12개를 슬라이드에 넣었습니다"}
	if err := store.SetGenerationNotes(ctx, deck.ID, owner.ID, said); err != nil {
		t.Fatalf("SetGenerationNotes: %v", err)
	}
	read, err := store.GetPresentation(ctx, deck.ID, owner.ID, false)
	if err != nil {
		t.Fatalf("read it back: %v", err)
	}
	kept := read.GenerationNotes
	if len(kept) != 2 || kept[0] != said[0] {
		t.Errorf("the deck kept %q", kept)
	}
	// Somebody else's deck is not theirs to write on.
	other, err := store.UpsertUser(ctx, "import-notes-other", "import-notes-other@ptium.test", "other", []string{}, false)
	if err != nil {
		t.Fatalf("other owner: %v", err)
	}
	if err := store.SetGenerationNotes(ctx, deck.ID, other.ID, []string{"남의 덱"}); err != nil {
		t.Fatalf("SetGenerationNotes: %v", err)
	}
	again, _ := store.GetPresentation(ctx, deck.ID, owner.ID, false)
	if still := again.GenerationNotes; len(still) != 2 {
		t.Errorf("somebody else wrote on this deck: %q", again.GenerationNotes)
	}
}
