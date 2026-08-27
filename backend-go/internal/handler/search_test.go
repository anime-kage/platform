package handler_test

// Title search. The old predicate was a plain `%q%`, so a short
// query matched inside longer words — searching "nana" returned
// "Osananajimi", which contains the letters but is not the thing anyone typed.

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

func seedTitle(t *testing.T, title string, english *string, score float64) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, `
		INSERT INTO anime (title, title_english, genres, studios, status, type, score)
		VALUES ($1, $2, '{}', '{}', 'completed', 'tv', $3)`, title, english, score); err != nil {
		t.Fatalf("seed %q: %v", title, err)
	}
}

func searchTitles(t *testing.T, c *client, q string) []string {
	t.Helper()
	status, body := c.do("GET", "/api/anime/search?limit=25&q="+url.QueryEscape(q), nil)
	c.mustStatus(200, status, body, "search "+q)
	var out []string
	for _, row := range body["data"].([]any) {
		out = append(out, row.(map[string]any)["title"].(string))
	}
	return out
}

func TestSearchMatchesWordStartsNotSubstrings(t *testing.T) {
	srv, _ := newTestServer(t)
	c := &client{t: t, base: srv.URL, ip: "10.14.0.1"}

	seedTitle(t, "Osananajimi ga Zettai ni Makenai Love Comedy", nil, 7.0)
	seedTitle(t, "Nana", nil, 8.0)
	seedTitle(t, "Nanatsu no Taizai", nil, 7.5)

	got := searchTitles(t, c, "nana")

	// the actual bug: "Osa(nana)jimi" is a substring hit, never a real one
	for _, title := range got {
		if title == "Osananajimi ga Zettai ni Makenai Love Comedy" {
			t.Error(`"nana" matched inside "Osananajimi" — substring, not a word`)
		}
	}
	// but genuine word-start matches must still be found
	want := map[string]bool{"Nana": false, "Nanatsu no Taizai": false}
	for _, title := range got {
		if _, ok := want[title]; ok {
			want[title] = true
		}
	}
	for title, found := range want {
		if !found {
			t.Errorf("%q missing from results for \"nana\": got %v", title, got)
		}
	}
}

// Punctuation is a word boundary too — a "space before it" check would miss
// this, which is why the predicate uses Postgres's \m anchor.
func TestSearchTreatsPunctuationAsAWordBoundary(t *testing.T) {
	srv, _ := newTestServer(t)
	c := &client{t: t, base: srv.URL, ip: "10.14.0.2"}

	seedTitle(t, "Re:Zero kara Hajimeru Isekai Seikatsu", nil, 8.2)

	got := searchTitles(t, c, "zero")
	if len(got) == 0 {
		t.Fatal(`"zero" found nothing — Re:Zero should match after the colon`)
	}
}

// A title that starts with the query outranks one that merely contains the
// word later, whatever the scores are. Regression for a NULL-ordering bug:
// title_english is NULL on most rows, `false OR NULL` is NULL, and DESC sorts
// NULLS FIRST — which inverted the ranking completely.
func TestSearchRanksPrefixMatchesFirst(t *testing.T) {
	srv, _ := newTestServer(t)
	c := &client{t: t, base: srv.URL, ip: "10.14.0.3"}

	// the distractor scores higher, so only relevance can put Naruto first
	seedTitle(t, "Naruto", nil, 7.9)
	seedTitle(t, "Zom 100: Zombie ni Naru made ni Shitai", nil, 9.5)

	got := searchTitles(t, c, "naru")
	if len(got) == 0 {
		t.Fatal("no results for \"naru\"")
	}
	if got[0] != "Naruto" {
		t.Errorf("first result = %q, want \"Naruto\" (prefix beats a higher score)", got[0])
	}
}

// Regex metacharacters in the query must not reach the regex engine — an
// unescaped "(" is an invalid pattern and would 500 the endpoint.
func TestSearchHandlesRegexMetacharacters(t *testing.T) {
	srv, _ := newTestServer(t)
	c := &client{t: t, base: srv.URL, ip: "10.14.0.4"}

	for _, q := range []string{"(", ")", "[", "*", "+", "a.b", "100%", `\`} {
		status, body := c.do("GET", "/api/anime/search?q="+url.QueryEscape(q), nil)
		c.mustStatus(200, status, body, fmt.Sprintf("search %q", q))
	}
}
