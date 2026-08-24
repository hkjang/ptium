package store

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// A person's first request is rarely alone: opening the workspace asks for the
// account, the decks and the templates at once, and every one of those creates
// the account if it is not there yet. The insert that loses that race used to
// collide with the unique index on the email — which the statement does not
// arbitrate on — and answer five hundred. About one first sign-in in three.
//
// Needs a database: set PTIUM_TEST_DSN to run it.
func TestAFirstSignInFromSeveralRequestsAtOnce(t *testing.T) {
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

	for round := 0; round < 12; round++ {
		email := fmt.Sprintf("first-sign-in-%d@ptium.test", round)
		if _, err := pool.Exec(ctx, `DELETE FROM users WHERE email=$1`, email); err != nil {
			t.Fatalf("clean up: %v", err)
		}
		var group sync.WaitGroup
		results := make([]error, 16)
		ids := make([]string, len(results))
		for index := range results {
			group.Add(1)
			go func(slot int) {
				defer group.Done()
				user, err := store.UpsertUser(ctx, "dev:"+email, email, email, []string{"user"}, false)
				results[slot], ids[slot] = err, user.ID
			}(index)
		}
		group.Wait()
		for slot, err := range results {
			if err != nil {
				t.Fatalf("round %d: request %d of %d failed: %v", round, slot+1, len(results), err)
			}
		}
		// And they are all the same person, not sixteen accounts.
		for _, id := range ids {
			if id != ids[0] {
				t.Fatalf("round %d: concurrent first requests made more than one account: %s and %s", round, ids[0], id)
			}
		}
		if _, err := pool.Exec(ctx, `DELETE FROM users WHERE email=$1`, email); err != nil {
			t.Fatalf("clean up: %v", err)
		}
	}
}
