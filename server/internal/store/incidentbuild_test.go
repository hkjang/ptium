package store

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hkjang/ptium/server/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

// An incident said what happened and when, never on what. Reading a critical
// panic in the error centre, an operator could not tell whether it belonged to
// the build in front of them or to one this deployment left months ago — and
// that is the whole question. The record now carries the build that saw it
// first and the build that saw it last, so a fault that survived an upgrade
// reads differently from one that stopped at a release.
//
// Needs a database: set PTIUM_TEST_DSN to run it.
func TestAnIncidentSaysWhichBuildSawIt(t *testing.T) {
	dsn := os.Getenv("PTIUM_TEST_DSN")
	if dsn == "" {
		t.Skip("set PTIUM_TEST_DSN to run the database-backed store tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()
	ctx := context.Background()
	run := fmt.Sprintf("build-%d", time.Now().UnixNano())

	older := New(pool).WithVersion("1.13.9")
	if err := older.CaptureIncident(ctx, model.Incident{Kind: "internal", Severity: "critical", Message: "panic while reading " + run}); err != nil {
		t.Fatalf("capture: %v", err)
	}
	first := findIncident(t, older, run)
	if first.FirstSeenVersion != "1.13.9" || first.LastSeenVersion != "1.13.9" {
		t.Fatalf("first capture recorded %q..%q, want 1.13.9 both", first.FirstSeenVersion, first.LastSeenVersion)
	}

	// The same fault on a later build moves the last-seen version forward and
	// leaves the first alone: that pair is what says it survived the upgrade.
	newer := New(pool).WithVersion("1.39.0")
	if err := newer.CaptureIncident(ctx, model.Incident{Kind: "internal", Severity: "critical", Message: "panic while reading " + run}); err != nil {
		t.Fatalf("recapture: %v", err)
	}
	again := findIncident(t, newer, run)
	if again.ID != first.ID {
		t.Fatalf("the repeat opened a second group (%s then %s)", first.ID, again.ID)
	}
	if again.OccurrenceCount != 2 {
		t.Fatalf("occurrence count is %d, want 2", again.OccurrenceCount)
	}
	if again.FirstSeenVersion != "1.13.9" {
		t.Errorf("first-seen build moved to %q; the build that first saw a fault does not change", again.FirstSeenVersion)
	}
	if again.LastSeenVersion != "1.39.0" {
		t.Errorf("last-seen build is %q, want the build that saw the repeat", again.LastSeenVersion)
	}

	// A group recorded before the product kept the build keeps an unknown first
	// version when it happens again. Blank means nobody knows which build saw it
	// first; filling it in with today's would say the fault started here.
	if _, err := pool.Exec(ctx, `UPDATE server_errors SET first_seen_version='',last_seen_version='' WHERE id=$1`, again.ID); err != nil {
		t.Fatalf("age the row: %v", err)
	}
	if err := newer.CaptureIncident(ctx, model.Incident{Kind: "internal", Severity: "critical", Message: "panic while reading " + run}); err != nil {
		t.Fatalf("recapture an aged row: %v", err)
	}
	aged := findIncident(t, newer, run)
	if aged.FirstSeenVersion != "" {
		t.Errorf("an older record was told it started on %q; unknown must stay unknown", aged.FirstSeenVersion)
	}
	if aged.LastSeenVersion != "1.39.0" {
		t.Errorf("last-seen build is %q, want the build that saw the repeat", aged.LastSeenVersion)
	}

	// A process built without a stamp records nothing rather than a version
	// that does not exist.
	unstamped := New(pool)
	if err := unstamped.CaptureIncident(ctx, model.Incident{Kind: "internal", Severity: "error", Message: "unstamped " + run}); err != nil {
		t.Fatalf("capture unstamped: %v", err)
	}
	blank := findIncident(t, unstamped, "unstamped "+run)
	if blank.LastSeenVersion != "" {
		t.Errorf("an unstamped build recorded %q", blank.LastSeenVersion)
	}
	for _, id := range []string{again.ID, blank.ID} {
		if _, err := pool.Exec(ctx, `DELETE FROM server_errors WHERE id=$1`, id); err != nil {
			t.Fatalf("clean up: %v", err)
		}
	}
}

func findIncident(t *testing.T, s *Store, needle string) model.Incident {
	t.Helper()
	incidents, _, err := s.ListIncidents(context.Background(), "", 200, 0)
	if err != nil {
		t.Fatalf("list incidents: %v", err)
	}
	for _, incident := range incidents {
		if strings.Contains(incident.Message, needle) {
			return incident
		}
	}
	t.Fatalf("no incident mentioning %q", needle)
	return model.Incident{}
}
