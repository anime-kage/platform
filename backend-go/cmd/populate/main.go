// Bulk-import anime from Jikan by MAL id. Replaces the old TS populate script.
//
//	go run ./cmd/populate                    # curated list of popular titles
//	go run ./cmd/populate -ids 5114,9253     # specific MAL ids
//
// Already-imported titles are skipped; the Jikan client rate-limits itself.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"

	"animekage/backend/internal/config"
	"animekage/backend/internal/db"
	"animekage/backend/internal/jikan"
	"animekage/backend/internal/repo"
)

// The same curated list the TS script shipped with: classics, recent hits,
// Ghibli, long-running shounen.
var popularMalIDs = []int{
	5114, 9253, 11061, 1535, 16498,
	40748, 44511, 50172, 48569, 39486,
	164, 430, 199,
	20, 21, 269, 30276, 35760,
	9969, 28977, 37510, 38000, 40456,
}

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	idsFlag := flag.String("ids", "", "comma-separated MAL ids (default: curated popular list)")
	flag.Parse()

	ids := popularMalIDs
	if *idsFlag != "" {
		ids = nil
		for _, s := range strings.Split(*idsFlag, ",") {
			id, err := strconv.Atoi(strings.TrimSpace(s))
			if err != nil {
				return fmt.Errorf("bad MAL id %q: %w", s, err)
			}
			ids = append(ids, id)
		}
	}

	_ = godotenv.Load()
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
	jk := jikan.NewClient()

	var imported, skipped, failed int
	for _, malID := range ids {
		if existing, err := r.AnimeByMalID(ctx, malID); err == nil {
			slog.Info("already imported", "malId", malID, "title", existing.Title)
			skipped++
			continue
		} else if err != repo.ErrNotFound {
			return fmt.Errorf("check mal_id %d: %w", malID, err)
		}

		data, err := jk.AnimeByID(malID)
		if err != nil {
			slog.Error("jikan fetch failed", "malId", malID, "err", err)
			failed++
			continue
		}
		a, err := r.InsertAnime(ctx, *data)
		if err != nil {
			slog.Error("insert failed", "malId", malID, "err", err)
			failed++
			continue
		}
		slog.Info("imported", "malId", malID, "title", a.Title)
		imported++
	}

	slog.Info("done", "imported", imported, "skipped", skipped, "failed", failed)
	if failed > 0 {
		return fmt.Errorf("%d of %d imports failed", failed, len(ids))
	}
	return nil
}
