// Source health checker: re-resolves every active extract source
// and records last_checked_at / last_ok. The stream endpoint sorts known-dead
// sources last, and the admin dashboard (5.6) reads the same columns — without
// this loop, source rot is invisible until users complain.
//
// Cron it alongside autoupdate, e.g. daily:
//
//	0 5 * * * cd /path/to/backend-go && go run ./cmd/healthcheck
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"animekage/backend/internal/config"
	"animekage/backend/internal/db"
	"animekage/backend/internal/repo"
	"animekage/backend/internal/resolver"
)

func main() {
	if err := run(); err != nil {
		slog.Error("healthcheck failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	url, err := config.DatabaseURL()
	if err != nil {
		return err
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()

	r := repo.New(pool)
	reg := resolver.Default()

	sources, err := r.AllActiveExtractSources(ctx)
	if err != nil {
		return fmt.Errorf("list sources: %w", err)
	}
	slog.Info("checking sources", "count", len(sources))

	healthy, dead, flipped := 0, 0, 0
	for _, src := range sources {
		if src.Provider == nil || src.ProviderRef == nil {
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		res, rerr := reg.Resolve(cctx, *src.Provider, *src.ProviderRef)
		if rerr == nil {
			// resolving proves the ref is well-formed; only a fetch proves the
			// stream is alive (the direct provider deliberately never fetches)
			rerr = resolver.Probe(cctx, res)
		}
		cancel()

		ok := rerr == nil
		if ok {
			healthy++
		} else {
			dead++
			slog.Warn("dead source", "id", src.ID, "provider", *src.Provider, "err", rerr)
		}
		if src.LastOK != nil && *src.LastOK != ok {
			flipped++
		}
		if err := r.RecordSourceHealth(ctx, src.ID, ok); err != nil {
			return fmt.Errorf("record health for %d: %w", src.ID, err)
		}
	}
	slog.Info("healthcheck done", "healthy", healthy, "dead", dead, "flipped", flipped)
	return nil
}
