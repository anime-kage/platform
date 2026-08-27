package repo

// White-box tests for the pure SQL-building logic: the shared anime/manga
// filter builder (the old backend had two copies that drifted — these tests
// pin the single implementation) and the comment-scope WHERE.

import (
	"strings"
	"testing"
)

func TestBuildTitleWhere(t *testing.T) {
	tests := []struct {
		name     string
		f        TitleFilters
		want     string
		wantArgs []any
	}{
		{
			name: "no filters",
			f:    TitleFilters{},
			want: "",
		},
		{
			// \m anchors at a word start, so "nana" no longer matches inside
			// "Osananajimi"
			name:     "text query hits all three title columns with one arg",
			f:        TitleFilters{Query: "frieren"},
			want:     ` WHERE (title ~* $1 OR title_english ~* $1 OR title_romanian ~* $1)`,
			wantArgs: []any{`\mfrieren`},
		},
		{
			// the query goes into a regex, so metacharacters must be escaped
			// or "(" becomes an invalid pattern and the query errors
			name:     "regex metacharacters in the query are escaped",
			f:        TitleFilters{Query: "re:zero (2016)"},
			want:     ` WHERE (title ~* $1 OR title_english ~* $1 OR title_romanian ~* $1)`,
			wantArgs: []any{`\mre:zero \(2016\)`},
		},
		{
			name:     "genres OR'd",
			f:        TitleFilters{Genres: []string{"Action", "Drama"}},
			want:     ` WHERE ($1 = ANY(genres) OR $2 = ANY(genres))`,
			wantArgs: []any{"Action", "Drama"},
		},
		{
			name:     "scalar filters AND'd in order",
			f:        TitleFilters{Year: 2024, Status: "airing", Type: "tv"},
			want:     ` WHERE year = $1 AND status = $2 AND type = $3`,
			wantArgs: []any{2024, "airing", "tv"},
		},
		{
			name:     "letter",
			f:        TitleFilters{Letter: "b"},
			want:     ` WHERE title ILIKE $1`,
			wantArgs: []any{"b%"},
		},
		{
			name: "letter 0-9",
			f:    TitleFilters{Letter: "0-9"},
			want: ` WHERE title ~ '^[0-9]'`,
		},
		{
			name: "letter other",
			f:    TitleFilters{Letter: "other"},
			want: ` WHERE title ~ '^[^a-zA-Z0-9]'`,
		},
		{
			// regression guard: a malicious letter value must not reach the SQL
			name: "letter is ignored unless a single ASCII letter",
			f:    TitleFilters{Letter: "'; DROP TABLE anime;--"},
			want: "",
		},
		{
			name: "combined keeps placeholder numbering consistent",
			f:    TitleFilters{Query: "one", Genres: []string{"Action"}, Year: 1999},
			want: ` WHERE (title ~* $1 OR title_english ~* $1 OR title_romanian ~* $1)` +
				` AND ($2 = ANY(genres)) AND year = $3`,
			wantArgs: []any{`\mone`, "Action", 1999},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var args []any
			got := buildTitleWhere(tt.f, &args)
			if got != tt.want {
				t.Errorf("where:\n got  %q\n want %q", got, tt.want)
			}
			if len(args) != len(tt.wantArgs) {
				t.Fatalf("args = %v, want %v", args, tt.wantArgs)
			}
			for i := range args {
				if args[i] != tt.wantArgs[i] {
					t.Errorf("args[%d] = %v, want %v", i, args[i], tt.wantArgs[i])
				}
			}
		})
	}
}

func TestTitleOrder(t *testing.T) {
	tests := []struct {
		sort, dir, wantSub string
	}{
		// no direction given = each sort's natural one
		{"score", "", "score DESC NULLS LAST"},
		{"title", "", "title ASC"},
		{"year", "", "year DESC NULLS LAST"},
		{"createdAt", "", "created_at DESC"},
		{"", "", "created_at DESC"},
		// reversed
		{"score", "asc", "score ASC NULLS LAST"},
		{"title", "desc", "title DESC"},
		{"year", "asc", "year ASC NULLS LAST"},
		// unknown values fall back on both axes
		{"bogus; DROP TABLE anime", "", "created_at DESC"},
		{"score", "; DROP TABLE anime", "score DESC NULLS LAST"},
		{"score", "ASC", "score DESC NULLS LAST"}, // case-sensitive by design
	}
	for _, tt := range tests {
		got := titleOrder(tt.sort, tt.dir)
		if !strings.Contains(got, tt.wantSub) {
			t.Errorf("titleOrder(%q, %q) = %q, want it to contain %q", tt.sort, tt.dir, got, tt.wantSub)
		}
		if strings.Contains(got, "DROP") {
			t.Errorf("titleOrder(%q, %q) leaked input into SQL: %q", tt.sort, tt.dir, got)
		}
	}
}

func TestExcerptOf(t *testing.T) {
	t.Run("collapses whitespace", func(t *testing.T) {
		got := excerptOf("  hello\n\n  world\t!  ")
		if got == nil || *got != "hello world !" {
			t.Errorf("got %v", got)
		}
	})
	t.Run("truncates at 90 runes with ellipsis", func(t *testing.T) {
		got := excerptOf(strings.Repeat("ă", 200)) // multibyte: must count runes, not bytes
		if got == nil {
			t.Fatal("nil")
		}
		if r := []rune(*got); len(r) != 91 || r[90] != '…' {
			t.Errorf("len=%d last=%q", len(r), string(r[len(r)-1]))
		}
	})
	t.Run("short text untouched", func(t *testing.T) {
		got := excerptOf("scurt")
		if got == nil || *got != "scurt" {
			t.Errorf("got %v", got)
		}
	})
	t.Run("blank is nil", func(t *testing.T) {
		if got := excerptOf("   "); got != nil {
			t.Errorf("got %v, want nil", *got)
		}
	})
}

func TestCommentScopeWhere(t *testing.T) {
	i := func(n int) *int { return &n }

	tests := []struct {
		name     string
		scope    CommentScope
		wantSubs []string
		wantArgs int
	}{
		{
			name:  "anime series-wide excludes every sub-scope",
			scope: CommentScope{AnimeID: i(52)},
			wantSubs: []string{
				"c.anime_id = $1", "c.is_deleted = false", "c.parent_id IS NULL",
				"c.episode_id IS NULL", "c.chapter_id IS NULL",
				"c.watchlist_id IS NULL", "c.readlist_id IS NULL",
			},
			wantArgs: 1,
		},
		{
			name:     "episode scope",
			scope:    CommentScope{AnimeID: i(52), EpisodeID: i(7)},
			wantSubs: []string{"c.anime_id = $1", "c.episode_id = $2"},
			wantArgs: 2,
		},
		{
			name:     "anime review scope wins over episode scope",
			scope:    CommentScope{AnimeID: i(52), EpisodeID: i(7), ReviewID: i(9)},
			wantSubs: []string{"c.watchlist_id = $2"},
			wantArgs: 2,
		},
		{
			name:     "manga review scope uses readlist",
			scope:    CommentScope{MangaID: i(3), ReviewID: i(9)},
			wantSubs: []string{"c.manga_id = $1", "c.readlist_id = $2"},
			wantArgs: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var args []any
			got := tt.scope.where(&args)
			for _, sub := range tt.wantSubs {
				if !strings.Contains(got, sub) {
					t.Errorf("where %q missing %q", got, sub)
				}
			}
			if len(args) != tt.wantArgs {
				t.Errorf("args = %v, want %d of them", args, tt.wantArgs)
			}
		})
	}
}
