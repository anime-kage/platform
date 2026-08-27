// Applies the numbered SQL migrations in ./migrations, in order, recording each
// one in the schema_migrations table so it runs exactly once.
//
//	go run ./cmd/migrate
//
// Migrations are .sql files embedded from cmd/migrate/migrations/, applied in
// lexical order and tracked in schema_migrations. To change the schema, add a
// new numbered file (0002_add_x.sql) — never edit an applied one.
//
// 0001_baseline.sql is the full schema as of the Go cutover (July 2026), taken
// from the same pg_dump snapshot as internal/handler/testdata/schema.sql. A
// database that predates this tool (already has the tables but no
// schema_migrations) is adopted: the baseline is recorded without being run.
package main

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"os"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"

	"animekage/backend/internal/config"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

const baseline = "0001_baseline.sql"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	_ = godotenv.Load()
	url, err := config.DatabaseURL()
	if err != nil {
		return err
	}

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer conn.Close(ctx)

	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS public.schema_migrations (
			version    text PRIMARY KEY,
			applied_at timestamptz NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied := map[string]bool{}
	rows, err := conn.Query(ctx, `SELECT version FROM public.schema_migrations`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return err
		}
		applied[v] = true
	}
	if rows.Err() != nil {
		return rows.Err()
	}

	// Adopt a pre-migration database: schema exists, ledger is empty.
	if !applied[baseline] {
		var hasSchema bool
		if err := conn.QueryRow(ctx, `SELECT EXISTS(
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = 'anime')`).Scan(&hasSchema); err != nil {
			return err
		}
		if hasSchema {
			if _, err := conn.Exec(ctx,
				`INSERT INTO public.schema_migrations (version) VALUES ($1)`, baseline); err != nil {
				return err
			}
			applied[baseline] = true
			slog.Info("adopted existing schema as baseline", "version", baseline)
		}
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	ran := 0
	for _, name := range names {
		if applied[name] {
			continue
		}
		sql, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		// Every migration runs with search_path pinned to public, because the
		// baseline is a pg_dump and pg_dump emits
		// `set_config('search_path', '', false)` — session-scoped, so on a
		// FRESH database it leaks out of the baseline into every file after it
		// and unqualified names (`ALTER TABLE anime`) stop resolving. This
		// never showed up in dev: the dev database predates this tool, so the
		// baseline is adopted rather than run, and the SET never executes.
		// SET LOCAL overrides the session value for this transaction only.
		if _, err := tx.Exec(ctx, `SET LOCAL search_path TO public`); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("pin search_path for %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO public.schema_migrations (version) VALUES ($1)`, name); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		slog.Info("applied", "version", name)
		ran++
	}

	if ran == 0 {
		slog.Info("up to date", "applied", len(applied))
	}
	return nil
}
