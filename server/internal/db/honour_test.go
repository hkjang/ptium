package db

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// A value stored before the API refused it must not survive the upgrade.
//
// Until this release the settings API took anything and answered 200, so a
// deployment can be holding a timeout of 99999 seconds — a number the readers
// have always ignored. Left there it shows an administrator a value their
// deployment does not use, and makes the whole section unsavable now that the
// API refuses what it will not honour.
//
// The other half matters just as much: a value an administrator chose, inside
// what this deployment honours, must be left exactly as they set it.
//
// Needs a database: set PTIUM_TEST_DSN to run it.
func TestAStoredSettingThatCannotBeHonouredIsPutBack(t *testing.T) {
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
	quiet := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	if err := Migrate(ctx, pool, quiet); err != nil {
		t.Fatalf("prepare: %v", err)
	}

	var before map[string]string
	read := func() map[string]string {
		rows, err := pool.Query(ctx, `SELECT key, value::text FROM app_settings`)
		if err != nil {
			t.Fatalf("read settings: %v", err)
		}
		defer rows.Close()
		values := map[string]string{}
		for rows.Next() {
			var key, value string
			if err := rows.Scan(&key, &value); err != nil {
				t.Fatalf("read setting: %v", err)
			}
			values[key] = value
		}
		return values
	}
	before = read()
	defer func() {
		for key, value := range before {
			_, _ = pool.Exec(ctx, `UPDATE app_settings SET value=$2::jsonb WHERE key=$1`, key, value)
		}
	}()

	set := func(key, value string) {
		if _, err := pool.Exec(ctx, `UPDATE app_settings SET value=$2::jsonb WHERE key=$1`, key, value); err != nil {
			t.Fatalf("store %s: %v", key, err)
		}
	}
	// What an upgrade can be holding, and one thing an administrator chose.
	set("ai.timeout_seconds", "99999")
	set("generation.repair_passes", "500")
	set("ai.reasoning", `"thinking-hard"`)
	set("generation.outline_pass", `"yes"`)
	set("ai.max_output_tokens", "16000")
	set("generation.default_audience", `"경영진과 의사결정자"`)

	if err := Migrate(ctx, pool, quiet); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	after := read()
	for key, want := range map[string]string{
		"ai.timeout_seconds":       "300",
		"generation.repair_passes": "3",
		"ai.reasoning":             `"auto"`,
		"generation.outline_pass":  "true",
	} {
		if after[key] != want {
			t.Errorf("%s was left at %s; this deployment honours %s", key, after[key], want)
		}
	}
	// Nothing an administrator actually chose may be touched.
	if after["ai.max_output_tokens"] != "16000" {
		t.Errorf("a chosen output limit became %s", after["ai.max_output_tokens"])
	}
	if after["generation.default_audience"] != `"경영진과 의사결정자"` {
		t.Errorf("a chosen audience became %s", after["generation.default_audience"])
	}
}
