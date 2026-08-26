package db

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/hkjang/ptium/server/internal/settings"
)

func Open(ctx context.Context, dsn string, logger *slog.Logger) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse database DSN: %w", err)
	}
	config.MaxConns = 20
	config.MinConns = 1
	config.MaxConnIdleTime = 5 * time.Minute
	config.MaxConnLifetime = time.Hour
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create database pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	if err := Migrate(ctx, pool, logger); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

func Migrate(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(72741001)`); err != nil {
		return fmt.Errorf("lock migrations: %w", err)
	}
	defer func() { _, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock(72741001)`) }()
	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version integer PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("prepare migration ledger: %w", err)
	}

	for i, statement := range migrations {
		version := i + 1
		if _, err := conn.Exec(ctx, statement); err != nil {
			return fmt.Errorf("apply migration %d: %w", version, err)
		}
		if _, err := conn.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES($1) ON CONFLICT DO NOTHING`, version); err != nil {
			return fmt.Errorf("record migration %d: %w", version, err)
		}
	}
	for key, setting := range defaultSettings {
		// The value is the administrator's and is never overwritten. The
		// description is documentation, and a correction to it that only reached
		// new installations would leave every existing one reading the old advice.
		_, err := conn.Exec(ctx, `INSERT INTO app_settings(key,value,sensitive,description) VALUES($1,$2::jsonb,$3,$4)
			ON CONFLICT(key) DO UPDATE SET description=EXCLUDED.description`,
			key, setting.Value, setting.Sensitive, setting.Description)
		if err != nil {
			return fmt.Errorf("seed setting %q: %w", key, err)
		}
	}
	if restored, err := honourStoredSettings(ctx, conn, logger); err != nil {
		return err
	} else if restored > 0 {
		logger.Info("settings reset to a value this deployment honours", "count", restored)
	}
	logger.Info("database migrations ready", "versions", len(migrations))
	return nil
}

// honourStoredSettings puts back the seeded value wherever a stored one cannot
// be acted on.
//
// Until this release the settings API took any value and answered 200, so a
// deployment upgrading into it can be holding a timeout of 99999 seconds or a
// repair count of 500 — values the readers have always ignored. Leaving them
// stored would show an administrator a number their deployment does not use,
// and would make the section they sit in impossible to save now that the API
// refuses what it will not honour. The bounds are the ones the readers apply.
func honourStoredSettings(ctx context.Context, conn interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, logger *slog.Logger) (int, error) {
	rows, err := conn.Query(ctx, `SELECT key, value FROM app_settings`)
	if err != nil {
		return 0, fmt.Errorf("read settings: %w", err)
	}
	stored := map[string]json.RawMessage{}
	for rows.Next() {
		var key string
		var value []byte
		if err := rows.Scan(&key, &value); err != nil {
			rows.Close()
			return 0, fmt.Errorf("read setting: %w", err)
		}
		stored[key] = append(json.RawMessage(nil), value...)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("read settings: %w", err)
	}

	restored := 0
	for key, value := range stored {
		seed, known := defaultSettings[key]
		if !known || settings.Honoured(key, value) {
			continue
		}
		if _, err := conn.Exec(ctx, `UPDATE app_settings SET value=$2::jsonb WHERE key=$1`, key, seed.Value); err != nil {
			return restored, fmt.Errorf("reset setting %q: %w", key, err)
		}
		logger.Warn("a stored setting could not be honoured and was put back",
			"key", key, "stored", string(value), "restored", seed.Value)
		restored++
	}
	return restored, nil
}
