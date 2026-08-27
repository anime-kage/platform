// Package repo is the data layer: hand-written SQL executed through pgx,
// scanned into model structs with scany. One file per domain.
package repo

import (
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

// animeCols selects the full anime row; prefix aliases (p="anime.") let the
// same list serve both top-level scans and joined nested-struct scans.
func animeCols(alias, prefix string) string {
	cols := ""
	for i, c := range []string{
		"id", "mal_id", "title", "title_english", "title_romanian", "synopsis",
		"synopsis_romanian", "genres", "studios", "status", "type", "episodes",
		"score", "year", "season", "image_url", "trailer_url", "broadcast_day",
		"broadcast_time", "slug", "created_at", "updated_at",
	} {
		if i > 0 {
			cols += ", "
		}
		cols += alias + "." + c
		if prefix != "" {
			cols += ` AS "` + prefix + c + `"`
		}
	}
	return cols
}

func mangaCols(alias, prefix string) string {
	cols := ""
	for i, c := range []string{
		"id", "mal_id", "title", "title_english", "title_romanian", "synopsis",
		"synopsis_romanian", "genres", "authors", "status", "type", "chapters",
		"volumes", "score", "year", "image_url", "slug", "created_at", "updated_at",
	} {
		if i > 0 {
			cols += ", "
		}
		cols += alias + "." + c
		if prefix != "" {
			cols += ` AS "` + prefix + c + `"`
		}
	}
	return cols
}
