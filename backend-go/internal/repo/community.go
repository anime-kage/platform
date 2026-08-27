package repo

// Data behind the /comunitate hub. Everything here is derived from real
// tables — members are real accounts, reviews are list entries with notes, the
// friends feed reads mutual follows, the team tab reads roles. No seed data.

import (
	"context"
	"sort"
	"strconv"
	"time"

	"github.com/georgysavva/scany/v2/pgxscan"

	"animekage/backend/internal/model"
)

func itoa(n int) string { return strconv.Itoa(n) }

// CommunityMembers lists real accounts for the Members tab, most active first
// (reviews, then followers). viewerID (0 = guest) personalises isFollowing.
func (r *Repo) CommunityMembers(ctx context.Context, viewerID, limit int) ([]model.CommunityMember, error) {
	rows := []model.CommunityMember{}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		SELECT u.id, u.username, u.avatar_url, u.bio, u.role,
		       (SELECT count(*)::int FROM follows f WHERE f.following_id = u.id) AS followers,
		       ((SELECT count(*) FROM watchlist w WHERE w.user_id = u.id AND w.notes <> '')
		        + (SELECT count(*) FROM readlist rl WHERE rl.user_id = u.id AND rl.notes <> ''))::int AS review_count,
		       (SELECT count(*)::int FROM user_lists ul WHERE ul.user_id = u.id AND ul.is_public) AS list_count,
		       CASE WHEN $1 = 0 THEN false
		            ELSE EXISTS(SELECT 1 FROM follows f WHERE f.follower_id = $1 AND f.following_id = u.id)
		       END AS is_following
		FROM users u
		WHERE u.is_banned = false
		ORDER BY review_count DESC, followers DESC, u.created_at
		LIMIT $2`, viewerID, limit)
	return rows, err
}

// CommunityReviews is the Reviews tab: every member's reviews (list entries
// carrying notes) across the whole platform, newest first.
func (r *Repo) CommunityReviews(ctx context.Context, limit int) ([]model.CommunityReview, error) {
	out := []model.CommunityReview{}
	for _, s := range []struct{ kind, table, fk, titleTable, commentFK string }{
		{"anime", "watchlist", "anime_id", "anime", "watchlist_id"},
		{"manga", "readlist", "manga_id", "manga", "readlist_id"},
	} {
		var rows []model.CommunityReview
		err := pgxscan.Select(ctx, r.pool, &rows, `
			SELECT '`+s.kind+`' AS kind, l.id AS entry_id, l.score, l.notes, l.updated_at,
			       u.id AS "user.id", u.username AS "user.username", u.avatar_url AS "user.avatar_url",
			       t.id AS "title.id", t.title AS "title.title",
			       t.title_romanian AS "title.title_romanian",
			       t.image_url AS "title.image_url", t.year AS "title.year",
			       (SELECT count(*)::int FROM comments c WHERE c.`+s.commentFK+` = l.id AND c.is_deleted = false) AS reply_count
			FROM `+s.table+` l
			JOIN users u ON u.id = l.user_id
			JOIN `+s.titleTable+` t ON t.id = l.`+s.fk+`
			WHERE l.notes IS NOT NULL AND l.notes <> ''
			ORDER BY l.updated_at DESC
			LIMIT $1`, limit)
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// FriendIDs returns the users who mutually follow the viewer (viewer follows
// them AND they follow viewer) — the "prieteni" whose activity is shown.
func (r *Repo) FriendIDs(ctx context.Context, userID int) ([]int, error) {
	ids := []int{}
	err := pgxscan.Select(ctx, r.pool, &ids, `
		SELECT f1.following_id
		FROM follows f1
		JOIN follows f2 ON f2.follower_id = f1.following_id AND f2.following_id = f1.follower_id
		WHERE f1.follower_id = $1`, userID)
	return ids, err
}

// FriendsActivity is the /comunitate feed: the same events as SiteActivity but
// restricted to mutual follows. An empty friend set is an empty feed, never the
// whole site.
func (r *Repo) FriendsActivity(ctx context.Context, friendIDs []int, limit int) ([]model.ActivityEvent, error) {
	if len(friendIDs) == 0 {
		return []model.ActivityEvent{}, nil
	}
	return r.activityFeed(ctx, friendIDs, limit)
}

// SiteActivity is the same feed with no author filter — what the whole
// community has been doing. This is what /home shows: a brand-new member has no
// friends yet, so a friends-only strip on the front page would be empty for
// exactly the people who most need to see the place is alive.
func (r *Repo) SiteActivity(ctx context.Context, limit int) ([]model.ActivityEvent, error) {
	return r.activityFeed(ctx, nil, limit)
}

// activityFeed builds the feed from real events: list updates (a review if the
// entry carries notes, otherwise a status change) and new forum threads. Merged
// newest-first. userIDs nil means every member; a non-empty slice restricts to
// those authors.
func (r *Repo) activityFeed(ctx context.Context, userIDs []int, limit int) ([]model.ActivityEvent, error) {
	out := []model.ActivityEvent{}

	// One filter for both queries: `$1 IS NULL` short-circuits to "everyone"
	// without building the SQL string twice.
	var authors any
	if userIDs != nil {
		authors = userIDs
	}

	// list activity (anime + manga), newest touch first
	for _, s := range []struct{ kind, table, fk, titleTable, prefix string }{
		{"anime", "watchlist", "anime_id", "anime", "/anime/"},
		{"manga", "readlist", "manga_id", "manga", "/manga/"},
	} {
		type listRow struct {
			model.CommentUser
			TitleID  int       `db:"title_id"`
			Title    string    `db:"title"`
			Status   string    `db:"status"`
			Score    *int      `db:"score"`
			HasNotes bool      `db:"has_notes"`
			At       time.Time `db:"at"`
		}
		var rows []listRow
		err := pgxscan.Select(ctx, r.pool, &rows, `
			SELECT u.id, u.username, u.avatar_url,
			       t.id AS title_id, coalesce(t.title_romanian, t.title) AS title,
			       l.status, l.score, (l.notes IS NOT NULL AND l.notes <> '') AS has_notes,
			       l.updated_at AS at
			FROM `+s.table+` l
			JOIN users u ON u.id = l.user_id
			JOIN `+s.titleTable+` t ON t.id = l.`+s.fk+`
			WHERE u.is_banned = false
			  -- Bulk imports are not activity: one member importing a backlog
			  -- would otherwise be the entire feed.
			  AND l.from_import = false
			  AND ($1::int[] IS NULL OR l.user_id = ANY($1::int[]))
			ORDER BY l.updated_at DESC
			LIMIT $2`, authors, limit)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			link := s.prefix + itoa(row.TitleID)
			ev := model.ActivityEvent{
				User: row.CommentUser, Target: row.Title, Link: &link, CreatedAt: row.At,
			}
			if row.HasNotes {
				ev.Type, ev.Verb = "review", "a scris o recenzie la"
				if row.Score != nil {
					m := "★ " + itoa(*row.Score)
					ev.Meta = &m
				}
			} else {
				ev.Type, ev.Verb = "list", listVerb(s.kind, row.Status)
			}
			out = append(out, ev)
		}
	}

	// new forum threads by friends
	{
		type thrRow struct {
			model.CommentUser
			ThreadID int       `db:"thread_id"`
			Title    string    `db:"title"`
			At       time.Time `db:"at"`
		}
		var rows []thrRow
		err := pgxscan.Select(ctx, r.pool, &rows, `
			SELECT u.id, u.username, u.avatar_url,
			       ft.id AS thread_id, ft.title, ft.created_at AS at
			FROM forum_threads ft
			JOIN users u ON u.id = ft.user_id
			WHERE u.is_banned = false
			  AND ($1::int[] IS NULL OR ft.user_id = ANY($1::int[]))
			ORDER BY ft.created_at DESC
			LIMIT $2`, authors, limit)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			link := "/comunitate/forum/" + itoa(row.ThreadID)
			out = append(out, model.ActivityEvent{
				Type: "thread", User: row.CommentUser, Verb: "a deschis un subiect",
				Target: row.Title, Link: &link, CreatedAt: row.At,
			})
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// listVerb renders a status change into a Romanian phrase.
func listVerb(kind, status string) string {
	switch status {
	case "completed":
		return "a terminat"
	case "watching":
		return "urmărește"
	case "reading":
		return "citește"
	case "plan-to-watch", "plan-to-read":
		return "a adăugat în listă"
	case "on-hold":
		return "a pus în așteptare"
	case "dropped":
		return "a abandonat"
	default:
		return "a actualizat"
	}
}

// CommunityStats: the two sidebar numbers.
func (r *Repo) CommunityStats(ctx context.Context) (model.CommunityStats, error) {
	var s model.CommunityStats
	err := r.pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM users),
		       (SELECT count(*) FROM subtitles WHERE status = 'published'),
		       (SELECT count(*) FROM anime) + (SELECT count(*) FROM manga),
		       (SELECT count(*) FROM users WHERE last_seen_at > now() - interval '5 minutes')`).
		Scan(&s.Members, &s.Subtitles, &s.Titles, &s.Online)
	return s, err
}

// HallOfFame ranks translators (by published releases they uploaded) and
// verifiers (by published releases they reviewed). window "month" restricts to
// the last 30 days of publishing; anything else is all-time. Both lists are
// derived purely from the releases pipeline, so they cover anime and manga
// alike and can never disagree with what actually went live.
func (r *Repo) HallOfFame(ctx context.Context, window string, limit int) (translators, verifiers []model.HallEntry, err error) {
	translators = []model.HallEntry{}
	verifiers = []model.HallEntry{}

	// updated_at is the publish moment for a terminal 'published' row (same
	// convention as RecentReleases); no params, so it's safe to concatenate.
	recent := ""
	if window == "month" {
		recent = " AND r.updated_at >= now() - interval '30 days'"
	}

	if err = pgxscan.Select(ctx, r.pool, &translators, `
		SELECT u.id AS user_id, u.username, u.avatar_url, u.role, count(*)::int AS count
		FROM releases r JOIN users u ON u.id = r.uploader_id
		WHERE r.state = 'published'`+recent+`
		GROUP BY u.id, u.username, u.avatar_url, u.role
		ORDER BY count DESC, u.username
		LIMIT $1`, limit); err != nil {
		return nil, nil, err
	}
	if err = pgxscan.Select(ctx, r.pool, &verifiers, `
		SELECT u.id AS user_id, u.username, u.avatar_url, u.role, count(*)::int AS count
		FROM releases r JOIN users u ON u.id = r.reviewer_id
		WHERE r.state = 'published' AND r.reviewer_id IS NOT NULL`+recent+`
		GROUP BY u.id, u.username, u.avatar_url, u.role
		ORDER BY count DESC, u.username
		LIMIT $1`, limit); err != nil {
		return nil, nil, err
	}
	return translators, verifiers, nil
}

// CommunityTeam lists staff (role above 'user') for the Echipă tab, admins
// first then by seniority, with the profile bits the card renders.
func (r *Repo) CommunityTeam(ctx context.Context) ([]model.TeamMember, error) {
	rows := []model.TeamMember{}
	err := pgxscan.Select(ctx, r.pool, &rows, `
		SELECT id, username, avatar_url, bio, role, created_at
		FROM users
		WHERE role <> 'user' AND is_banned = false
		ORDER BY array_position(ARRAY['admin','moderator','coordinator','verifier','translator'], role), created_at`)
	return rows, err
}
