package mail

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hkjang/ptium/server/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The table is written by one process and read by an administrator later, so
// what goes in has to come back out with its outcome and its counts.
//
// Needs a database: set PTIUM_TEST_DSN to run it.
func TestTheDatabaseLedgerKeepsSentAndFailedAlike(t *testing.T) {
	dsn := os.Getenv("PTIUM_TEST_DSN")
	if dsn == "" {
		t.Skip("set PTIUM_TEST_DSN to run the database-backed ledger test")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ledger := NewDatabaseLedger(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)
	sent := Delivery{ID: uuid.NewString(), Event: EventGenerationCompleted, Recipient: "a@corp.example", Subject: "덱 완성", PresentationID: uuid.NewString(), ActorID: "", CreatedAt: now}
	failed := Delivery{ID: uuid.NewString(), Event: EventTest, Recipient: "b@corp.example", Subject: "시험", ActorID: uuid.NewString(), CreatedAt: now.Add(time.Second)}
	for _, delivery := range []Delivery{sent, failed} {
		if err := ledger.Record(ctx, delivery); err != nil {
			t.Fatalf("record: %v", err)
		}
	}
	if err := ledger.Complete(ctx, sent.ID, StatusSent, 1, "", now.Add(time.Second)); err != nil {
		t.Fatalf("complete sent: %v", err)
	}
	if err := ledger.Complete(ctx, failed.ID, StatusFailed, 2, "SMTP connection failed: refused", now.Add(2*time.Second)); err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	page, err := ledger.List(ctx, StatusFailed, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if page.Status[StatusSent] < 1 || page.Status[StatusFailed] < 1 || page.Total < 2 {
		t.Fatalf("counts = %+v", page)
	}
	found := false
	for _, item := range page.Items {
		if item.Status != StatusFailed {
			t.Fatalf("filtered listing carries %+v", item)
		}
		if item.ID == failed.ID {
			found = item.Attempts == 2 && item.ErrorMessage == "SMTP connection failed: refused" && item.ActorID == failed.ActorID
		}
	}
	if !found {
		t.Fatalf("the failed delivery did not come back as recorded: %+v", page.Items)
	}
	all, err := ledger.List(ctx, "", 200)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	for _, item := range all.Items {
		if item.ID == sent.ID && (item.Status != StatusSent || item.PresentationID != sent.PresentationID || item.Attempts != 1) {
			t.Fatalf("the sent delivery came back as %+v", item)
		}
	}
}
