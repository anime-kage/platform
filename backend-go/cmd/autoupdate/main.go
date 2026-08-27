// Keep the catalog fresh from Jikan. Run periodically (cron). Replaces the
// old TS auto-update-episodes script.
//
//	go run ./cmd/autoupdate            # episodes (default)
//	go run ./cmd/autoupdate refresh    # re-sync airing/upcoming anime metadata
//	go run ./cmd/autoupdate seasonal   # import the current season's new titles
//	go run ./cmd/autoupdate all        # everything
//
// Deliberate changes from the TS script:
//   - anime.episodes is NOT overwritten with the DB row count (the TS script
//     did, which made the watchlist auto-complete fire at "episodes aired so
//     far"). The true total comes from Jikan metadata via `refresh`.
//   - `refresh` also populates broadcast_day/broadcast_time, which the
//     /calendar page needs.
//   - The LibreTranslate call was not ported; Romanian titles/synopses are
//     curated by translators, not machine-translated.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/joho/godotenv"

	"animekage/backend/internal/anilist"
	"animekage/backend/internal/config"
	"animekage/backend/internal/db"
	"animekage/backend/internal/jikan"
	"animekage/backend/internal/repo"
	"animekage/backend/internal/slug"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	command := "episodes"
	if len(os.Args) > 1 {
		command = os.Args[1]
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

	switch command {
	case "episodes":
		return updateEpisodes(ctx, r, jk)
	case "refresh":
		return refreshMetadata(ctx, r, jk)
	case "seasonal":
		return importSeasonal(ctx, r, jk)
	case "banners":
		return fetchBanners(ctx, r)
	case "relations":
		return syncRelations(ctx, r)
	case "backfill":
		return backfillEpisodeMeta(ctx, r, jk)
	case "slugs":
		return fillSlugs(ctx, r)
	case "all":
		// `seasonal` is deliberately NOT here. It used to be, and it grew the
		// catalog on its own every night: it imports every title of the current
		// season, which is where the sequels-with-no-prequels came from
		// (Mushoku Tensei III, Grand Blue Season 3, Youjo Senki II, …). The
		// catalog is curated by hand now — a series enters when it is being
		// translated, not because it happens to be airing. Run `autoupdate
		// seasonal` explicitly if you ever want that behaviour back.
		if err := updateEpisodes(ctx, r, jk); err != nil {
			return err
		}
		if err := refreshMetadata(ctx, r, jk); err != nil {
			return err
		}
		if err := fetchBanners(ctx, r); err != nil {
			return err
		}
		// cheap and idempotent (no network) — keeps a hand-added title's URL
		// pretty without anyone having to remember a command
		if err := fillSlugs(ctx, r); err != nil {
			return err
		}
		// last, so a title added to the catalog by hand since the previous run
		// picks up its relations here
		return syncRelations(ctx, r)
	default:
		return fmt.Errorf("unknown command %q (episodes | refresh | banners | relations | backfill | seasonal | all)", command)
	}
}

// updateEpisodes adds newly-aired episodes for every airing anime.
func updateEpisodes(ctx context.Context, r *repo.Repo, jk *jikan.Client) error {
	airing, err := r.AiringAnime(ctx)
	if err != nil {
		return err
	}
	slog.Info("checking episodes", "anime", len(airing))

	for _, a := range airing {
		if a.MalID == nil {
			slog.Warn("skipping, no MAL id", "title", a.Title)
			continue
		}
		eps, err := jk.AnimeEpisodes(*a.MalID)
		if err != nil {
			slog.Error("jikan episodes failed", "title", a.Title, "err", err)
			continue
		}
		existing, err := r.EpisodeNumbers(ctx, a.ID)
		if err != nil {
			return err
		}

		added, filled := 0, 0
		for _, ep := range eps {
			if existing[ep.Number] {
				// Backfill rather than skip. Existing rows used to be left
				// untouched for ever, so an episode created before its title
				// was known — or created by hand with the title box empty —
				// stayed nameless permanently. This only fills gaps; a title
				// someone typed is never overwritten (see FillEpisodeMeta).
				// This path is always Jikan, so the flags are always real.
				changed, err := r.FillEpisodeMeta(ctx, a.ID, ep.Number, ep.Title, ep.Aired, &ep.Filler, &ep.Recap)
				if err != nil {
					slog.Error("fill episode meta failed", "title", a.Title, "episode", ep.Number, "err", err)
					continue
				}
				if changed {
					filled++
				}
				continue
			}
			// No "Episode 5" placeholder title: a NULL renders as "Episodul 5"
			// in the UI, which is the same information in Romanian, and it
			// leaves the field visibly empty so a later sync can fill it.
			_, err := r.CreateEpisode(ctx, a.ID, ep.Number, repo.EpisodeInput{
				Title:    ep.Title,
				AirDate:  ep.Aired,
				IsFiller: &ep.Filler,
				IsRecap:  &ep.Recap,
			})
			if err == repo.ErrExists {
				continue
			}
			if err != nil {
				slog.Error("insert episode failed", "title", a.Title, "episode", ep.Number, "err", err)
				continue
			}
			added++
		}
		if added > 0 || filled > 0 {
			slog.Info("episodes synced", "title", a.Title, "added", added, "filled", filled)
		}
	}
	return nil
}

// refreshMetadata re-syncs airing/upcoming anime from Jikan: status flips
// (upcoming→airing→completed), score, episode totals, broadcast day/time.
func refreshMetadata(ctx context.Context, r *repo.Repo, jk *jikan.Client) error {
	airing, err := r.AiringAnime(ctx)
	if err != nil {
		return err
	}
	slog.Info("refreshing metadata", "anime", len(airing))

	for _, a := range airing {
		if a.MalID == nil {
			continue
		}
		data, err := jk.AnimeByID(*a.MalID)
		if err != nil {
			slog.Error("jikan fetch failed", "title", a.Title, "err", err)
			continue
		}
		if _, err := r.UpdateAnimeFromJikan(ctx, a.ID, *data); err != nil {
			slog.Error("update failed", "title", a.Title, "err", err)
			continue
		}
		if data.Status != a.Status {
			slog.Info("status changed", "title", a.Title, "from", a.Status, "to", data.Status)
		}
	}
	return nil
}

// importSeasonal adds new titles from the current Jikan season (first page,
// like the TS script — the season's headliners).
//
// NO LONGER PART OF `all`, and therefore no longer part of the nightly cron.
// It is an explicit, manual tool: running it grows the catalog with whatever is
// airing, which is the opposite of a hand-curated catalog. Left in place
// because importing a season on purpose is still occasionally useful.
func importSeasonal(ctx context.Context, r *repo.Repo, jk *jikan.Client) error {
	now := time.Now()
	season := seasonOf(now.Month())
	slog.Info("importing season", "year", now.Year(), "season", season)

	list, _, err := jk.SeasonalAnime(now.Year(), season, 1)
	if err != nil {
		return err
	}

	var added, skipped int
	for _, data := range list {
		if _, err := r.AnimeByMalID(ctx, data.MalID); err == nil {
			skipped++
			continue
		} else if err != repo.ErrNotFound {
			return err
		}
		a, err := r.InsertAnime(ctx, data)
		if err != nil {
			slog.Error("insert failed", "malId", data.MalID, "err", err)
			continue
		}
		slog.Info("added", "title", a.Title)
		added++
	}
	slog.Info("seasonal done", "added", added, "skipped", skipped)
	return nil
}

func seasonOf(m time.Month) string {
	switch {
	case m <= time.March:
		return "winter"
	case m <= time.June:
		return "spring"
	case m <= time.September:
		return "summer"
	default:
		return "fall"
	}
}

// fetchBanners fills anime/manga banner_url from AniList.
//
// AniList is the only source that has this art at all, and it takes 50 ids
// per request — so a full catalog backfill is a handful of calls, not one per
// title. A partial index means a repeat run scans only what is still missing,
// which is what makes this safe to hang off `all` on a cron.
// fillSlugs gives every title a URL slug, so /anime/36 can be /anime/91-days.
//
// Idempotent and cheap (no network): it only touches rows whose slug is NULL, so
// it is safe to re-run after an import and safe to leave in `all`. Existing
// slugs are never regenerated — a slug is a URL, and rewriting it on a title
// edit would break every link anyone had shared.
func fillSlugs(ctx context.Context, r *repo.Repo) error {
	for _, kind := range []struct {
		manga bool
		label string
	}{{false, "anime"}, {true, "manga"}} {
		rows, err := r.TitlesMissingSlug(ctx, kind.manga)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			slog.Info("slugs already complete", "kind", kind.label)
			continue
		}

		var set, skipped int
		for _, row := range rows {
			base := slug.Make(row.Title)
			if base == "" {
				// A title made only of symbols has no usable slug; the numeric
				// id keeps working, so this is a skip and not an error.
				skipped++
				continue
			}
			// On collision, disambiguate with the MAL id — stable across re-runs,
			// unlike a "-2" counter that depends on insertion order.
			candidate := base
			if err := r.SetSlug(ctx, kind.manga, row.ID, candidate); err == repo.ErrExists {
				if row.MalID != nil {
					candidate = fmt.Sprintf("%s-%d", base, *row.MalID)
				} else {
					candidate = fmt.Sprintf("%s-%d", base, row.ID)
				}
				if err := r.SetSlug(ctx, kind.manga, row.ID, candidate); err != nil {
					slog.Error("slug failed", "kind", kind.label, "id", row.ID, "err", err)
					skipped++
					continue
				}
			} else if err != nil {
				slog.Error("slug failed", "kind", kind.label, "id", row.ID, "err", err)
				skipped++
				continue
			}
			set++
		}
		slog.Info("slugs filled", "kind", kind.label, "set", set, "skipped", skipped)
	}
	return nil
}

// backfillEpisodeMeta fills episode titles, air dates and filler/recap marks for
// EVERY anime in the catalog, not just the airing ones.
//
// `episodes` (the nightly step) polls `AiringAnime`, i.e. status airing/upcoming.
// That is right for its job — finding newly aired episodes — but it means a
// completed series added by hand never gets its episode metadata from anywhere.
// 91 Days was the case that surfaced it: twelve episodes created through the
// admin form with the title box empty, and nothing that would ever fill them.
//
// Manual only. It walks the whole catalog and paginates per series, so it is far
// too expensive to run nightly, and titles rarely change once known. The
// per-series button in /admin/anime/{id} covers the common single case; this is
// the bulk tool for after a large import.
func backfillEpisodeMeta(ctx context.Context, r *repo.Repo, jk *jikan.Client) error {
	titles, err := r.AllAnimeWithMalID(ctx)
	if err != nil {
		return err
	}
	al := anilist.NewClient()
	slog.Info("backfilling episode metadata", "anime", len(titles))

	var touched, added, filled, failed int
	for _, a := range titles {
		if a.MalID == nil {
			continue // the SQL filters these out, but the type still allows nil
		}
		eps, err := jk.AnimeEpisodes(*a.MalID)
		fromAniList := false
		if err != nil || len(eps) == 0 {
			// MAL fails per-entry, not just globally: 91 Days and NANA 504 on
			// every attempt while other series answer fine, so retrying alone
			// would never fill them. AniList's streamingEpisodes carry the
			// titles — but no filler marks and no air dates, hence the flag.
			titles, alErr := al.EpisodeTitlesByMal(*a.MalID)
			if alErr != nil || len(titles) == 0 {
				slog.Warn("no episode source", "title", a.Title, "jikan", err, "anilist", alErr)
				failed++
				continue
			}
			eps = eps[:0]
			for num, t := range titles {
				title := t
				eps = append(eps, jikan.EpisodeData{Number: num, Title: &title})
			}
			fromAniList = true
			slog.Info("using anilist titles", "title", a.Title, "episodes", len(eps))
		}
		existing, err := r.EpisodeNumbers(ctx, a.ID)
		if err != nil {
			return err
		}

		var a1, f1 int
		for _, ep := range eps {
			if ep.Number <= 0 {
				continue
			}
			var filler, recap *bool
			if !fromAniList {
				filler, recap = &ep.Filler, &ep.Recap
			}
			if existing[ep.Number] {
				changed, err := r.FillEpisodeMeta(ctx, a.ID, ep.Number, ep.Title, ep.Aired, filler, recap)
				if err != nil {
					slog.Error("fill failed", "title", a.Title, "episode", ep.Number, "err", err)
					continue
				}
				if changed {
					f1++
				}
				continue
			}
			// Only fill in what is missing from series we already track; do not
			// invent episodes for a series nobody has started. An episode row
			// with no sources renders as "în curând", which is honest.
			if _, err := r.CreateEpisode(ctx, a.ID, ep.Number, repo.EpisodeInput{
				Title: ep.Title, AirDate: ep.Aired,
				IsFiller: filler, IsRecap: recap,
			}); err != nil && err != repo.ErrExists {
				slog.Error("insert failed", "title", a.Title, "episode", ep.Number, "err", err)
				continue
			} else if err == nil {
				a1++
			}
		}
		if a1 > 0 || f1 > 0 {
			slog.Info("backfilled", "title", a.Title, "added", a1, "filled", f1)
			touched++
			added += a1
			filled += f1
		}
	}
	slog.Info("backfill done", "series touched", touched, "added", added, "filled", filled, "unreachable", failed)
	return nil
}

// syncRelations refreshes the season chain / franchise graph from AniList.
//
// Every catalog title is re-asked each run, not just new ones: relations are a
// property of the *pair*, so importing one series changes what its neighbours
// should link to, and a "only titles added since last time" filter would leave
// the older half of each new pair stale. At 50 ids per request the whole
// catalog is a couple of calls, so there is nothing to save by being clever.
//
// Nothing here imports a series. An edge pointing at a title we do not have is
// stored as a bare MAL id and resolves itself if that title ever arrives.
func syncRelations(ctx context.Context, r *repo.Repo) error {
	malIDs, err := r.AnimeMalIDs(ctx)
	if err != nil {
		return err
	}
	if len(malIDs) == 0 {
		slog.Info("no anime to relate")
		return nil
	}

	al := anilist.NewClient()
	byMal, err := al.RelationsByMal(malIDs)
	if err != nil {
		// Same posture as banners: keep whatever came back, log the rest.
		slog.Warn("relation fetch incomplete", "err", err)
	}

	var titles, edges int
	for mal, rels := range byMal {
		a, err := r.AnimeByMalID(ctx, mal)
		if err != nil {
			continue // catalog changed under us mid-run
		}
		rows := make([]repo.RelationRow, 0, len(rels))
		for _, rel := range rels {
			rows = append(rows, repo.RelationRow{
				AnimeID: a.ID, Relation: rel.Kind, RelatedMalID: rel.RelatedMalID,
			})
		}
		if err := r.ReplaceAnimeRelations(ctx, a.ID, rows); err != nil {
			slog.Error("store relations failed", "malId", mal, "err", err)
			continue
		}
		titles++
		edges += len(rows)
	}
	slog.Info("relations synced", "asked", len(malIDs), "titles", titles, "edges", edges)
	return nil
}

func fetchBanners(ctx context.Context, r *repo.Repo) error {
	al := anilist.NewClient()

	for _, kind := range []struct {
		gql   string
		manga bool
		label string
	}{{"ANIME", false, "anime"}, {"MANGA", true, "manga"}} {
		malIDs, err := r.TitlesMissingBanner(ctx, kind.manga, 500)
		if err != nil {
			return err
		}
		if len(malIDs) == 0 {
			slog.Info("banners already complete", "kind", kind.label)
			continue
		}

		banners, err := al.BannersByMal(malIDs, kind.gql)
		if err != nil {
			// partial results are still worth storing — the next run picks
			// up whatever this one missed
			slog.Warn("banner fetch incomplete", "kind", kind.label, "err", err)
		}
		n, err := r.SetBanners(ctx, banners, kind.manga, malIDs...)
		if err != nil {
			return err
		}
		slog.Info("banners updated", "kind", kind.label,
			"asked", len(malIDs), "found", len(banners), "stored", n)
	}
	return nil
}
