package store

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hkjang/ptium/server/internal/model"
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

// A rewrite that does not work leaves the deck somebody already had.
//
// Needs a database: set PTIUM_TEST_DSN to run it.
func TestAFailedRewriteLeavesTheDeckItHad(t *testing.T) {
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
	owner, err := store.UpsertUser(ctx, "failed-rewrite", "failed-rewrite@ptium.test", "rewrite", []string{}, false)
	if err != nil {
		t.Fatalf("owner: %v", err)
	}
	deck, err := store.CreatePresentation(ctx, owner.ID, PresentationInput{
		Title: "다듬기 실패", Prompt: "브리프", Theme: "slate-classic",
		Language: "ko", Audience: "general", Tone: "professional", SlideCount: 3})
	if err != nil {
		t.Fatalf("deck: %v", err)
	}
	defer func() { _, _ = pool.Exec(ctx, `DELETE FROM presentations WHERE id=$1`, deck.ID) }()
	if err := store.CompleteGeneration(ctx, deck.ID, "", []byte(`{}`), []model.Slide{
		{Position: 1, Title: "한 장", Content: []byte(`{}`)}}, "# 한 장", nil); err != nil {
		// Completing needs the lease it was claimed under; this deck was never
		// claimed, so write the slide directly.
		if _, err := pool.Exec(ctx, `INSERT INTO slides(id,presentation_id,position,title,content) VALUES(gen_random_uuid(),$1,1,'한 장','{}'::jsonb)`, deck.ID); err != nil {
			t.Fatalf("give it a slide: %v", err)
		}
	}
	var lease string
	if err := pool.QueryRow(ctx, `UPDATE presentations SET status='generating',generation_lease=gen_random_uuid()
		WHERE id=$1 RETURNING generation_lease::text`, deck.ID).Scan(&lease); err != nil {
		t.Fatalf("hand it to a worker: %v", err)
	}

	kept, err := store.FailRewrite(ctx, deck.ID, lease, "덱을 다시 쓰려면 연결된 AI 모델이 필요합니다")
	if err != nil || !kept {
		t.Fatalf("FailRewrite() = %v, %v", kept, err)
	}
	after, err := store.GetPresentation(ctx, deck.ID, owner.ID, false)
	if err != nil {
		t.Fatalf("read it back: %v", err)
	}
	if after.Status != "completed" {
		t.Errorf("a deck that already had slides reads as %q after a failed rewrite", after.Status)
	}
	if len(after.Slides) == 0 {
		t.Error("the deck lost its slides")
	}
	if len(after.GenerationNotes) == 0 || !strings.Contains(after.GenerationNotes[len(after.GenerationNotes)-1], "AI 모델") {
		t.Errorf("the reason is not in the deck's notes: %q", after.GenerationNotes)
	}

	// A deck with nothing in it has nothing to keep: that failure is a failure.
	empty, err := store.CreatePresentation(ctx, owner.ID, PresentationInput{
		Title: "빈 덱", Prompt: "브리프", Theme: "slate-classic",
		Language: "ko", Audience: "general", Tone: "professional", SlideCount: 3})
	if err != nil {
		t.Fatalf("deck: %v", err)
	}
	defer func() { _, _ = pool.Exec(ctx, `DELETE FROM presentations WHERE id=$1`, empty.ID) }()
	var emptyLease string
	if err := pool.QueryRow(ctx, `UPDATE presentations SET status='generating',generation_lease=gen_random_uuid()
		WHERE id=$1 RETURNING generation_lease::text`, empty.ID).Scan(&emptyLease); err != nil {
		t.Fatalf("hand it to a worker: %v", err)
	}
	if kept, err := store.FailRewrite(ctx, empty.ID, emptyLease, "무엇이든"); err != nil || kept {
		t.Errorf("an empty deck was kept as though it had something: %v, %v", kept, err)
	}
}

// Every link this deployment has handed out, and closing one.
//
// A link reaches somebody with no account here, and only the deck's owner could
// see their own: an operator asked what of theirs is readable outside had no
// way to answer, and no way to close a link left open by somebody who has gone.
//
// Needs a database: set PTIUM_TEST_DSN to run it.
func TestAnOperatorSeesEveryLinkAndCanCloseOne(t *testing.T) {
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
	owner, err := store.UpsertUser(ctx, "share-oversight", "share-oversight@ptium.test", "링크 주인", []string{}, false)
	if err != nil {
		t.Fatalf("owner: %v", err)
	}
	deck, err := store.CreatePresentation(ctx, owner.ID, PresentationInput{
		Title: "감독 점검 덱", Prompt: "브리프", Theme: "slate-classic",
		Language: "ko", Audience: "general", Tone: "professional", SlideCount: 3})
	if err != nil {
		t.Fatalf("deck: %v", err)
	}
	defer func() { _, _ = pool.Exec(ctx, `DELETE FROM presentations WHERE id=$1`, deck.ID) }()
	openLink, _, err := store.CreateShare(ctx, deck.ID, owner.ID, "감독 열린 링크", nil)
	if err != nil {
		t.Fatalf("open link: %v", err)
	}
	past := time.Now().Add(-2 * time.Hour)
	goneLink, _, err := store.CreateShare(ctx, deck.ID, owner.ID, "감독 기한 링크", &past)
	if err != nil {
		t.Fatalf("dated link: %v", err)
	}

	mine := func(state string) map[string]OpenShare {
		shares, total, err := store.ListAllShares(ctx, SharesFilter{State: state, Search: "감독"}, 100, 0)
		if err != nil {
			t.Fatalf("list %q: %v", state, err)
		}
		if total < len(shares) {
			t.Errorf("the list says %d of %d", len(shares), total)
		}
		found := map[string]OpenShare{}
		for _, one := range shares {
			found[one.ID] = one
		}
		return found
	}
	all := mine("")
	if one, ok := all[openLink.ID]; !ok {
		t.Error("a link with no day on it is not in the deployment's list")
	} else {
		if one.State != "open" {
			t.Errorf("a link with no day reads as %q", one.State)
		}
		if one.DeckTitle != "감독 점검 덱" || one.OwnerEmail != "share-oversight@ptium.test" {
			t.Errorf("the list does not say whose deck it is: %q / %q", one.DeckTitle, one.OwnerEmail)
		}
	}
	if one, ok := all[goneLink.ID]; !ok || one.State != "expired" {
		t.Errorf("a link whose day has passed reads as %v", ok && one.State == "expired")
	}
	if _, ok := mine("open")[goneLink.ID]; ok {
		t.Error("a link whose day has passed is listed among the open ones")
	}

	// Closing is the operator's, whoever made it, and it says what it did.
	closed, didClose, err := store.CloseShare(ctx, openLink.ID)
	if err != nil || !didClose {
		t.Fatalf("CloseShare() = %v, %v", didClose, err)
	}
	if closed.State != "revoked" {
		t.Errorf("a link just closed reads as %q", closed.State)
	}
	if _, again, err := store.CloseShare(ctx, openLink.ID); err != nil || again {
		t.Errorf("closing an already closed link answered %v (err %v)", again, err)
	}
	if _, ok := mine("revoked")[openLink.ID]; !ok {
		t.Error("a closed link is not among the closed ones")
	}
}

// An organisation's standard can be its own file.
//
// A deployment that uploads the company template and makes it the standard
// means that template. The theme a deck falls back to used to be read only as a
// shipped design key, so a deck asked for with no template chosen landed in
// Ptium Slate Classic while the settings said otherwise.
//
// Needs a database: set PTIUM_TEST_DSN to run it.
func TestTheStandardCanBeAnUploadedTemplate(t *testing.T) {
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
	owner, err := store.UpsertUser(ctx, "standard-template", "standard@ptium.test", "표준", []string{}, false)
	if err != nil {
		t.Fatalf("owner: %v", err)
	}
	var uploaded string
	if err := pool.QueryRow(ctx, `INSERT INTO templates(owner_id,name,description,filename,kind,scope,size_bytes,checksum,manifest,data)
		VALUES($1,'회사 표준','', 'standard.pptx','uploaded','shared',1,'deadbeef','{"layouts":[]}'::jsonb,'\\x00')
		RETURNING id::text`, owner.ID).Scan(&uploaded); err != nil {
		t.Fatalf("upload: %v", err)
	}
	defer func() { _, _ = pool.Exec(ctx, `DELETE FROM templates WHERE id=$1`, uploaded) }()

	got, err := store.DefaultTemplateID(ctx, owner.ID, uploaded)
	if err != nil {
		t.Fatalf("DefaultTemplateID: %v", err)
	}
	if got != uploaded {
		t.Errorf("the standard resolved to %s, not the uploaded template", got)
	}
	// A shipped design key still resolves to that design.
	shipped, err := store.DefaultTemplateID(ctx, owner.ID, "slate-classic")
	if err != nil || shipped == uploaded {
		t.Errorf("a built-in standard resolved to %s (err %v)", shipped, err)
	}
	// And a uuid that is nobody's template falls back rather than failing.
	missing, err := store.DefaultTemplateID(ctx, owner.ID, "11111111-2222-3333-4444-555555555555")
	if err != nil || missing == "" {
		t.Errorf("an unknown standard resolved to %q (err %v)", missing, err)
	}
}
