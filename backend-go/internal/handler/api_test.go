// Integration tests: the real router against a real Postgres.
//
// They need a DEDICATED test database (they drop and recreate its schema):
//
//	docker exec anime-kage-postgres-dev psql -U dev -d postgres \
//	  -c "CREATE DATABASE anime_kage_test OWNER dev"
//	TEST_DATABASE_URL='postgresql://dev:dev_password@localhost:5432/anime_kage_test' go test ./...
//
// Without TEST_DATABASE_URL they skip, so a plain `go test ./...` stays green.
// testdata/schema.sql is a pg_dump snapshot of the dev schema — regenerate it
// after schema changes:
//
//	docker exec anime-kage-postgres-dev pg_dump -U dev --schema-only \
//	  --no-owner --no-privileges anime_kage_dev | sed '/^\\/d' > internal/handler/testdata/schema.sql
package handler_test

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"animekage/backend/internal/auth"
	"animekage/backend/internal/config"
	"animekage/backend/internal/db"
	"animekage/backend/internal/handler"
	"animekage/backend/internal/httpx"
)

//go:embed testdata/schema.sql
var schemaSQL string

const testSecret = "integration-test-secret"

// fixture ids filled in by newTestServer's seed
type fixtures struct {
	animeID   int // 12 episodes
	anime2ID  int // no episode count
	mangaID   int // 5 chapters
	episodeID int // episode 1 of animeID
}

// testMDChapter is the one chapter UUID the fake MangaDex server knows.
const testMDChapter = "11111111-2222-3333-4444-555555555555"

func newTestServer(t *testing.T) (*httptest.Server, fixtures) {
	return newTestServerCfg(t, nil)
}

// newTestServerCfg is newTestServer with a hook to adjust the config before
// the handler is built — for switches like INVITE_ONLY that change behaviour
// server-wide and so cannot be toggled per request.
func newTestServerCfg(t *testing.T, tweak func(*config.Config)) (*httptest.Server, fixtures) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration tests (see file header)")
	}
	if !strings.Contains(dsn, "test") {
		t.Fatalf("refusing to reset %q: TEST_DATABASE_URL must point at a database with 'test' in its name", dsn)
	}
	ctx := context.Background()

	// schema reset on a throwaway connection (the dump alters search_path)
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect for schema reset: %v", err)
	}
	if _, err := conn.Exec(ctx,
		`DROP SCHEMA IF EXISTS public CASCADE; DROP SCHEMA IF EXISTS drizzle CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	if _, err := conn.Exec(ctx, schemaSQL); err != nil {
		t.Fatalf("apply testdata/schema.sql: %v", err)
	}
	conn.Close(ctx)

	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect pool: %v", err)
	}
	t.Cleanup(pool.Close)

	var fx fixtures
	err = pool.QueryRow(ctx, `
		INSERT INTO anime (title, genres, studios, status, type, episodes, score, year)
		VALUES ('Test Show', '{Action,Drama}', '{Ghibli}', 'airing', 'tv', 12, 8.5, 2024)
		RETURNING id`).Scan(&fx.animeID)
	if err != nil {
		t.Fatalf("seed anime: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO anime (title, genres, studios, status, type)
		VALUES ('Second Show', '{Comedy}', '{}', 'completed', 'movie')
		RETURNING id`).Scan(&fx.anime2ID); err != nil {
		t.Fatalf("seed anime2: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO manga (title, genres, authors, status, type, chapters)
		VALUES ('Test Manga', '{Action}', '{Author}', 'publishing', 'manga', 5)
		RETURNING id`).Scan(&fx.mangaID); err != nil {
		t.Fatalf("seed manga: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO episodes (anime_id, episode_number, title)
		VALUES ($1, 1, 'Seed Episode') RETURNING id`, fx.animeID).Scan(&fx.episodeID); err != nil {
		t.Fatalf("seed episode: %v", err)
	}

	// fake AniSkip: knows MAL id 5114, 404s everything else
	aniskipFake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v2/skip-times/5114/") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"found":true,"results":[
				{"interval":{"startTime":10.5,"endTime":95.5},"skipType":"op"},
				{"interval":{"startTime":1290,"endTime":1380},"skipType":"ed"}]}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(aniskipFake.Close)

	// fake Anthropic API for auto-translate: parses the lines out
	// of the user message and echoes them back prefixed with "RO: "
	anthropicFake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		text := req.Messages[0].Content[0].Text
		var payload struct {
			Lines []struct {
				Index int    `json:"index"`
				Text  string `json:"text"`
			} `json:"lines"`
		}
		_ = json.Unmarshal([]byte(text[strings.Index(text, "\n")+1:]), &payload)
		lines := make([]map[string]any, len(payload.Lines))
		for i, l := range payload.Lines {
			lines[i] = map[string]any{"index": l.Index, "text": "RO: " + l.Text}
		}
		outJSON, _ := json.Marshal(map[string]any{"lines": lines})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_test", "type": "message", "role": "assistant",
			"model":       "claude-sonnet-5",
			"content":     []map[string]any{{"type": "text", "text": string(outJSON)}},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	t.Cleanup(anthropicFake.Close)

	// fake MangaDex: knows one chapter UUID and serves its "CDN"
	// images itself, so the at-home response's baseUrl points back at it
	var mdURL string
	mangadexFake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/at-home/server/"+testMDChapter:
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"result":"ok","baseUrl":%q,"chapter":{"hash":"h1","data":["1.png","2.png"]}}`, mdURL)
		case strings.HasPrefix(r.URL.Path, "/data/h1/"):
			w.Header().Set("Content-Type", "image/png")
			fmt.Fprint(w, "PNGDATA")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	mdURL = mangadexFake.URL
	t.Cleanup(mangadexFake.Close)

	cfg := &config.Config{
		UploadsDir: t.TempDir(), AniskipBaseURL: aniskipFake.URL, StagingDir: t.TempDir(),
		AnthropicAPIKey: "test-key", AnthropicBaseURL: anthropicFake.URL,
		MangadexBaseURL: mangadexFake.URL,
		// httptest connects over loopback, so trusting it is what lets each
		// client below claim its own IP via X-Forwarded-For — the same
		// mechanism Caddy uses in production, with the same trust boundary.
		TrustedProxies: mustTrust(t, "127.0.0.0/8,::1/128"),
	}
	if tweak != nil {
		tweak(cfg)
	}
	h := handler.New(pool, auth.NewManager(testSecret, time.Hour), cfg)
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)
	return srv, fx
}

func mustTrust(t *testing.T, raw string) []netip.Prefix {
	t.Helper()
	p, err := httpx.ParseTrustedProxies(raw)
	if err != nil {
		t.Fatalf("ParseTrustedProxies(%q): %v", raw, err)
	}
	return p
}

// client is a tiny API driver. Each flow gets its own fake IP so per-IP rate
// limits can't couple unrelated subtests.
type client struct {
	t     *testing.T
	base  string
	token string
	ip    string
}

func (c *client) do(method, path string, body any) (int, map[string]any) {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, rd)
	if err != nil {
		c.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", c.ip)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// mustStatus fails the test loudly with the response body when the status is wrong.
func (c *client) mustStatus(want, got int, body map[string]any, what string) {
	c.t.Helper()
	if got != want {
		c.t.Fatalf("%s: status %d, want %d (body: %v)", what, got, want, body)
	}
}

var userSeq int

// signup registers a fresh user and logs the client in.
func (c *client) signup() (username string) {
	c.t.Helper()
	userSeq++
	username = fmt.Sprintf("qa_user_%d_%d", os.Getpid(), userSeq)
	status, body := c.do("POST", "/api/auth/register", map[string]any{
		"username": username, "email": username + "@test.local", "password": "longpassword123",
	})
	c.mustStatus(201, status, body, "register")
	c.token = body["token"].(string)
	return username
}

func data(body map[string]any) map[string]any {
	d, _ := body["data"].(map[string]any)
	return d
}

// ── Flows ─────────────────────────────────────────────────────────────────────

func TestAPI(t *testing.T) {
	srv, fx := newTestServer(t)

	t.Run("Auth", func(t *testing.T) {
		c := &client{t: t, base: srv.URL, ip: "10.1.0.1"}

		status, body := c.do("POST", "/api/auth/register", map[string]any{
			"username": "shortpw", "email": "shortpw@test.local", "password": "short"})
		c.mustStatus(400, status, body, "weak password")

		username := c.signup()

		status, body = c.do("POST", "/api/auth/register", map[string]any{
			"username": "other_name", "email": username + "@test.local", "password": "longpassword123"})
		c.mustStatus(400, status, body, "duplicate email")

		status, body = c.do("POST", "/api/auth/register", map[string]any{
			"username": username, "email": "fresh@test.local", "password": "longpassword123"})
		c.mustStatus(400, status, body, "duplicate username")

		status, body = c.do("POST", "/api/auth/login", map[string]any{
			"email": username + "@test.local", "password": "wrongpassword!"})
		c.mustStatus(401, status, body, "wrong password")

		status, body = c.do("POST", "/api/auth/login", map[string]any{
			"email": username + "@test.local", "password": "longpassword123"})
		c.mustStatus(200, status, body, "login")

		status, body = c.do("GET", "/api/auth/me", nil)
		c.mustStatus(200, status, body, "me")
		me := body["user"].(map[string]any)
		if me["username"] != username || me["email"] != username+"@test.local" {
			t.Errorf("me = %v", me)
		}
		if _, leaked := me["passwordHash"]; leaked {
			t.Fatal("passwordHash leaked in /me response")
		}

		anon := &client{t: t, base: srv.URL, ip: "10.1.0.1"}
		status, body = anon.do("GET", "/api/auth/me", nil)
		anon.mustStatus(401, status, body, "me without token")
	})

	t.Run("WatchlistPartialMerge", func(t *testing.T) {
		c := &client{t: t, base: srv.URL, ip: "10.1.0.2"}
		c.signup()

		// create with a review (notes) and a score
		status, body := c.do("POST", "/api/users/me/watchlist", map[string]any{
			"animeId": fx.animeID, "status": "watching", "score": 8, "notes": "recenzia mea"})
		c.mustStatus(201, status, body, "create entry")

		// progress-only update must NOT wipe notes or score — reviews live in notes
		status, body = c.do("PUT", fmt.Sprintf("/api/users/me/watchlist/%d", fx.animeID),
			map[string]any{"episodesWatched": 3})
		c.mustStatus(200, status, body, "progress update")

		status, body = c.do("GET", fmt.Sprintf("/api/users/me/watchlist/%d", fx.animeID), nil)
		c.mustStatus(200, status, body, "read back")
		d := data(body)
		if d["notes"] != "recenzia mea" || d["score"] != float64(8) || d["episodesWatched"] != float64(3) {
			t.Fatalf("partial merge broken: notes=%v score=%v progress=%v", d["notes"], d["score"], d["episodesWatched"])
		}

		// reaching the episode total auto-completes the entry
		status, body = c.do("PUT", fmt.Sprintf("/api/users/me/watchlist/%d", fx.animeID),
			map[string]any{"episodesWatched": 12})
		c.mustStatus(200, status, body, "finish")
		if d := data(body); d["status"] != "completed" || d["completedAt"] == nil {
			t.Errorf("auto-complete: status=%v completedAt=%v", d["status"], d["completedAt"])
		}

		// progress deltas feed the history chart
		status, body = c.do("GET", "/api/users/me/history?days=7", nil)
		c.mustStatus(200, status, body, "history")
		days := body["data"].([]any)
		var episodesToday float64
		if len(days) > 0 {
			episodesToday = days[len(days)-1].(map[string]any)["episodes"].(float64)
		}
		if episodesToday != 12 {
			t.Errorf("history episodes today = %v, want 12 (3 then +9)", episodesToday)
		}

		// validation
		status, body = c.do("POST", "/api/users/me/watchlist", map[string]any{
			"animeId": fx.animeID, "status": "bogus"})
		c.mustStatus(400, status, body, "invalid status")
		status, body = c.do("POST", "/api/users/me/watchlist", map[string]any{
			"animeId": fx.animeID, "status": "watching", "score": 11})
		c.mustStatus(400, status, body, "score out of range")
		status, body = c.do("POST", "/api/users/me/watchlist", map[string]any{
			"animeId": 999999, "status": "watching"})
		c.mustStatus(404, status, body, "unknown anime")

		// remove
		status, body = c.do("DELETE", fmt.Sprintf("/api/users/me/watchlist/%d", fx.animeID), nil)
		c.mustStatus(200, status, body, "remove")
		status, body = c.do("GET", fmt.Sprintf("/api/users/me/watchlist/%d", fx.animeID), nil)
		c.mustStatus(404, status, body, "read after remove")
	})

	t.Run("Readlist", func(t *testing.T) {
		c := &client{t: t, base: srv.URL, ip: "10.1.0.3"}
		c.signup()

		status, body := c.do("POST", "/api/users/me/readlist", map[string]any{
			"mangaId": fx.mangaID, "status": "reading", "chaptersRead": 5})
		c.mustStatus(201, status, body, "create entry")
		if d := data(body); d["status"] != "completed" {
			t.Errorf("auto-complete at chapter total: status = %v", d["status"])
		}
	})

	t.Run("CommentsScopingAndThreads", func(t *testing.T) {
		c := &client{t: t, base: srv.URL, ip: "10.1.0.4"}
		author := c.signup()
		animePath := fmt.Sprintf("/api/anime/%d/comments", fx.animeID)

		status, body := c.do("POST", animePath, map[string]any{"content": "series-wide comment"})
		c.mustStatus(201, status, body, "post series comment")
		root := data(body)
		rootID := int(root["id"].(float64))
		if u := root["user"].(map[string]any); u["username"] != author {
			t.Errorf("author = %v, want %v (avatar/user must come from DB, not JWT)", u, author)
		}

		status, body = c.do("POST", animePath, map[string]any{"content": "episode talk", "episodeId": fx.episodeID})
		c.mustStatus(201, status, body, "post episode-scoped comment")

		// series-wide list must not include the episode-scoped comment
		status, body = c.do("GET", animePath, nil)
		c.mustStatus(200, status, body, "series list")
		for _, raw := range body["data"].([]any) {
			if raw.(map[string]any)["content"] == "episode talk" {
				t.Error("episode-scoped comment leaked into the series-wide list")
			}
		}

		// thread: reply, then reply-to-reply joins the same root
		status, body = c.do("POST", fmt.Sprintf("/api/comments/%d/reply", rootID),
			map[string]any{"content": "first reply"})
		c.mustStatus(201, status, body, "reply")
		replyID := int(data(body)["id"].(float64))

		status, body = c.do("POST", fmt.Sprintf("/api/comments/%d/reply", replyID),
			map[string]any{"content": "nested reply"})
		c.mustStatus(201, status, body, "nested reply")
		if d := data(body); d["replyToUsername"] != author {
			t.Errorf("nested reply replyToUsername = %v, want %v", d["replyToUsername"], author)
		}

		status, body = c.do("GET", fmt.Sprintf("/api/comments/%d/replies", rootID), nil)
		c.mustStatus(200, status, body, "replies list")
		replies := body["data"].([]any)
		if len(replies) != 2 {
			t.Fatalf("thread size = %d, want 2 (flat, whole thread)", len(replies))
		}
		if replies[0].(map[string]any)["content"] != "first reply" {
			t.Error("replies not oldest-first")
		}

		// repliesCount lives on the root and counts the whole thread
		status, body = c.do("GET", animePath, nil)
		c.mustStatus(200, status, body, "series list again")
		for _, raw := range body["data"].([]any) {
			cm := raw.(map[string]any)
			if int(cm["id"].(float64)) == rootID && cm["repliesCount"] != float64(2) {
				t.Errorf("root repliesCount = %v, want 2", cm["repliesCount"])
			}
		}

		// votes: add → toggle off → switch
		votePath := fmt.Sprintf("/api/comments/%d/vote", rootID)
		status, body = c.do("POST", votePath, map[string]any{"voteType": "like"})
		c.mustStatus(200, status, body, "vote")
		if body["voteType"] != "like" {
			t.Errorf("vote add: %v", body)
		}
		status, body = c.do("POST", votePath, map[string]any{"voteType": "like"})
		if body["voteType"] != nil {
			t.Errorf("vote toggle off: %v", body)
		}
		status, body = c.do("POST", votePath, map[string]any{"voteType": "dislike"})
		if body["voteType"] != "dislike" {
			t.Errorf("vote switch: %v", body)
		}
		_ = status

		// only the author can edit/delete
		other := &client{t: t, base: srv.URL, ip: "10.1.0.5"}
		other.signup()
		status, body = other.do("PUT", fmt.Sprintf("/api/comments/%d", rootID),
			map[string]any{"content": "hijacked"})
		other.mustStatus(404, status, body, "edit someone else's comment")

		status, body = c.do("PUT", fmt.Sprintf("/api/comments/%d", rootID),
			map[string]any{"content": "edited"})
		c.mustStatus(200, status, body, "edit own comment")

		status, body = c.do("DELETE", fmt.Sprintf("/api/comments/%d", rootID), nil)
		c.mustStatus(200, status, body, "delete own comment")
		status, body = c.do("GET", animePath, nil)
		for _, raw := range body["data"].([]any) {
			if int(raw.(map[string]any)["id"].(float64)) == rootID {
				t.Error("soft-deleted comment still listed")
			}
		}

		// a review-thread comment must point at a review of THIS anime
		status, body = c.do("POST", "/api/users/me/watchlist", map[string]any{
			"animeId": fx.anime2ID, "status": "watching", "notes": "review on the other show"})
		c.mustStatus(201, status, body, "review entry")
		reviewID := int(data(body)["id"].(float64))
		status, body = c.do("POST", animePath, map[string]any{"content": "hm", "reviewId": reviewID})
		c.mustStatus(404, status, body, "review comment on the wrong anime")
	})

	t.Run("RoleGating", func(t *testing.T) {
		c := &client{t: t, base: srv.URL, ip: "10.1.0.6"}
		c.signup()
		epPath := fmt.Sprintf("/api/anime/%d/episodes", fx.animeID)

		for _, tc := range []struct{ method, path string }{
			{"POST", "/api/anime/import/1"},
			{"PUT", fmt.Sprintf("/api/anime/%d/update", fx.animeID)},
			{"POST", epPath},
			{"POST", fmt.Sprintf("/api/manga/%d/chapters", fx.mangaID)},
		} {
			status, body := c.do(tc.method, tc.path, map[string]any{"episodeNumber": 1, "chapterNumber": 1})
			c.mustStatus(403, status, body, "plain user on "+tc.method+" "+tc.path)
		}

		anon := &client{t: t, base: srv.URL, ip: "10.1.0.6"}
		status, body := anon.do("POST", epPath, map[string]any{"episodeNumber": 1})
		anon.mustStatus(401, status, body, "unauthenticated write")
	})

	t.Run("EpisodesChaptersAsAdmin", func(t *testing.T) {
		srv2, fx2 := srv, fx
		c := &client{t: t, base: srv2.URL, ip: "10.1.0.7"}
		username := c.signup()

		// promote to admin directly in the DB, then re-login so the JWT carries the role
		promote(t, username, "admin")
		status, body := c.do("POST", "/api/auth/login", map[string]any{
			"email": username + "@test.local", "password": "longpassword123"})
		c.mustStatus(200, status, body, "re-login as admin")
		c.token = body["token"].(string)

		epPath := fmt.Sprintf("/api/anime/%d/episodes", fx2.animeID)
		status, body = c.do("POST", epPath, map[string]any{
			"episodeNumber": 2, "title": "Pilot", "airDate": "2024-01-06", "duration": 24})
		c.mustStatus(201, status, body, "create episode")
		if d := data(body); d["airDate"] != "2024-01-06" {
			t.Errorf(`airDate = %v, want plain "2024-01-06" (frontend expects date strings)`, d["airDate"])
		}

		status, body = c.do("POST", epPath, map[string]any{"episodeNumber": 2})
		c.mustStatus(409, status, body, "duplicate episode")

		// partial update keeps the untouched fields
		status, body = c.do("PUT", epPath+"/2", map[string]any{"title": "Renamed"})
		c.mustStatus(200, status, body, "update episode")
		if d := data(body); d["title"] != "Renamed" || d["duration"] != float64(24) {
			t.Errorf("partial episode update: %v", data(body))
		}

		epID := int(data(body)["id"].(float64))
		status, body = c.do("POST", fmt.Sprintf("/api/episodes/%d/links", epID),
			map[string]any{"hostingUrl": "https://cdn.example/ep1", "quality": "1080p"})
		c.mustStatus(201, status, body, "add link")
		linkID := int(data(body)["id"].(float64))
		if d := data(body); d["language"] != "ro" || d["kind"] != "embed" || d["priority"] != float64(0) {
			t.Errorf("link defaults = %v, want language=ro kind=embed priority=0", d)
		}

		// + 3.1 validation: bad URLs and half-specified extract sources
		status, body = c.do("POST", fmt.Sprintf("/api/episodes/%d/links", epID),
			map[string]any{"hostingUrl": "http://cdn.example/ep1"})
		c.mustStatus(400, status, body, "http link rejected")
		status, body = c.do("POST", fmt.Sprintf("/api/episodes/%d/links", epID),
			map[string]any{"hostingUrl": "https://cdn.example/ep1", "kind": "extract"})
		c.mustStatus(400, status, body, "extract without provider rejected")

		// a higher-priority extract source sorts first
		status, body = c.do("POST", fmt.Sprintf("/api/episodes/%d/links", epID),
			map[string]any{"hostingUrl": "https://filemoon.example/e/abc", "kind": "extract",
				"provider": "filemoon", "providerRef": "abc", "priority": 10})
		c.mustStatus(201, status, body, "add extract link")
		extractID := int(data(body)["id"].(float64))

		status, body = c.do("GET", epPath+"/2", nil)
		c.mustStatus(200, status, body, "episode with links")
		links := data(body)["links"].([]any)
		if len(links) != 2 {
			t.Fatalf("links = %v", links)
		}
		if first := links[0].(map[string]any); first["kind"] != "extract" || first["provider"] != "filemoon" {
			t.Errorf("priority ordering: first link = %v, want the extract source", first)
		}
		status, body = c.do("DELETE", fmt.Sprintf("/api/links/%d", extractID), nil)
		c.mustStatus(200, status, body, "delete extract link")

		status, body = c.do("DELETE", fmt.Sprintf("/api/links/%d", linkID), nil)
		c.mustStatus(200, status, body, "delete link")
		status, body = c.do("DELETE", epPath+"/2", nil)
		c.mustStatus(200, status, body, "delete episode")

		// fractional chapter numbers survive the round trip
		chPath := fmt.Sprintf("/api/manga/%d/chapters", fx2.mangaID)
		status, body = c.do("POST", chPath, map[string]any{"chapterNumber": 10.5, "title": "Extra"})
		c.mustStatus(201, status, body, "create chapter 10.5")
		status, body = c.do("GET", chPath+"/10.5", nil)
		c.mustStatus(200, status, body, "get chapter 10.5")
		if d := data(body); d["title"] != "Extra" {
			t.Errorf("chapter = %v", d)
		}
	})

	t.Run("StreamResolve", func(t *testing.T) {
		srv2, fx2 := srv, fx
		admin := &client{t: t, base: srv2.URL, ip: "10.1.0.12"}
		username := admin.signup()
		promote(t, username, "admin")
		status, body := admin.do("POST", "/api/auth/login", map[string]any{
			"email": username + "@test.local", "password": "longpassword123"})
		admin.mustStatus(200, status, body, "re-login as admin")
		admin.token = body["token"].(string)

		// no extract sources yet → the player falls back to embeds
		guest := &client{t: t, base: srv2.URL, ip: "10.1.0.13"}
		status, body = guest.do("GET", fmt.Sprintf("/api/episodes/%d/stream", fx2.episodeID), nil)
		guest.mustStatus(404, status, body, "stream without sources")

		// a dead direct ref (http) resolves to nothing; a good one wins by priority
		status, body = admin.do("POST", fmt.Sprintf("/api/episodes/%d/links", fx2.episodeID),
			map[string]any{"hostingUrl": "https://cdn.example/page", "kind": "extract",
				"provider": "direct", "providerRef": "http://cdn.example/broken.m3u8", "priority": 20})
		admin.mustStatus(201, status, body, "add broken extract source")
		status, body = admin.do("POST", fmt.Sprintf("/api/episodes/%d/links", fx2.episodeID),
			map[string]any{"hostingUrl": "https://cdn.example/page", "kind": "extract",
				"provider": "direct", "providerRef": "https://cdn.example/stream/master.m3u8", "priority": 10})
		admin.mustStatus(201, status, body, "add good extract source")

		status, body = guest.do("GET", fmt.Sprintf("/api/episodes/%d/stream", fx2.episodeID), nil)
		guest.mustStatus(200, status, body, "resolve stream")
		stream := data(body)["stream"].(map[string]any)
		if stream["manifestUrl"] != "https://cdn.example/stream/master.m3u8" || stream["kind"] != "hls" {
			t.Errorf("stream = %v, want the healthy hls source (broken one skipped)", stream)
		}
		if src := data(body)["source"].(map[string]any); src["provider"] != "direct" {
			t.Errorf("source = %v", src)
		}

		//: published subtitle tracks ride along with the stream, RO first
		status, body = guest.do("POST", fmt.Sprintf("/api/episodes/%d/subtitles", fx2.episodeID),
			map[string]any{"language": "ro", "url": "https://cdn.example/subs/ep1.ro.vtt"})
		guest.mustStatus(401, status, body, "guest cannot add subtitles")
		status, body = admin.do("POST", fmt.Sprintf("/api/episodes/%d/subtitles", fx2.episodeID),
			map[string]any{"language": "ro", "url": "http://cdn.example/subs/ep1.ro.vtt"})
		admin.mustStatus(400, status, body, "http subtitle URL rejected")
		status, body = admin.do("POST", fmt.Sprintf("/api/episodes/%d/subtitles", fx2.episodeID),
			map[string]any{"language": "ro", "label": "Română", "url": "https://cdn.example/subs/ep1.ro.vtt"})
		admin.mustStatus(201, status, body, "add ro subtitle")
		subID := int(data(body)["id"].(float64))
		status, body = admin.do("POST", fmt.Sprintf("/api/episodes/%d/subtitles", fx2.episodeID),
			map[string]any{"language": "ro", "url": "https://cdn.example/subs/other.ro.vtt"})
		admin.mustStatus(409, status, body, "duplicate published ro subtitle")
		status, body = admin.do("POST", fmt.Sprintf("/api/episodes/%d/subtitles", fx2.episodeID),
			map[string]any{"language": "en", "url": "https://cdn.example/subs/ep1.en.vtt"})
		admin.mustStatus(201, status, body, "add en subtitle")

		status, body = guest.do("GET", fmt.Sprintf("/api/episodes/%d/stream", fx2.episodeID), nil)
		guest.mustStatus(200, status, body, "stream with subtitles")
		subs, _ := data(body)["stream"].(map[string]any)["subtitles"].([]any)
		if len(subs) != 2 {
			t.Fatalf("stream subtitles = %v, want ro+en", subs)
		}
		if first := subs[0].(map[string]any); first["language"] != "ro" || first["label"] != "Română" {
			t.Errorf("first track = %v, want RO first with its label", first)
		}

		status, body = guest.do("GET", fmt.Sprintf("/api/episodes/%d/subtitles", fx2.episodeID), nil)
		guest.mustStatus(200, status, body, "list subtitles")
		if got := body["data"].([]any); len(got) != 2 {
			t.Errorf("subtitle list = %v", got)
		}

		status, body = admin.do("DELETE", fmt.Sprintf("/api/subtitles/%d", subID), nil)
		admin.mustStatus(200, status, body, "delete subtitle")
	})

	t.Run("SkipMarks", func(t *testing.T) {
		srv2, fx2 := srv, fx
		admin := &client{t: t, base: srv2.URL, ip: "10.1.0.14"}
		username := admin.signup()
		promote(t, username, "admin")
		status, body := admin.do("POST", "/api/auth/login", map[string]any{
			"email": username + "@test.local", "password": "longpassword123"})
		admin.mustStatus(200, status, body, "re-login as admin")
		admin.token = body["token"].(string)
		guest := &client{t: t, base: srv2.URL, ip: "10.1.0.15"}

		// no MAL id on the fixture anime → no AniSkip lookup, both marks null
		status, body = guest.do("GET", fmt.Sprintf("/api/episodes/%d/skip", fx2.episodeID), nil)
		guest.mustStatus(200, status, body, "skip without marks")
		if d := data(body); d["intro"] != nil || d["outro"] != nil {
			t.Errorf("skip = %v, want both null", d)
		}

		// give the second anime a MAL id AniSkip knows and an episode
		ctx := context.Background()
		conn, err := pgx.Connect(ctx, os.Getenv("TEST_DATABASE_URL"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(ctx,
			`UPDATE anime SET mal_id = 5114 WHERE id = $1`, fx2.anime2ID); err != nil {
			t.Fatal(err)
		}
		conn.Close(ctx)
		status, body = admin.do("POST", fmt.Sprintf("/api/anime/%d/episodes", fx2.anime2ID),
			map[string]any{"episodeNumber": 1})
		admin.mustStatus(201, status, body, "create episode on MAL-linked anime")
		epID := int(data(body)["id"].(float64))

		// AniSkip hit is returned and cached into skip_marks
		status, body = guest.do("GET", fmt.Sprintf("/api/episodes/%d/skip", epID), nil)
		guest.mustStatus(200, status, body, "skip via aniskip")
		intro, _ := data(body)["intro"].(map[string]any)
		outro, _ := data(body)["outro"].(map[string]any)
		if intro == nil || intro["start"] != 10.5 || intro["end"] != 95.5 || outro == nil {
			t.Fatalf("aniskip marks = %v", data(body))
		}

		// manual marks are role-gated and override the cached aniskip value
		status, body = guest.do("POST", fmt.Sprintf("/api/episodes/%d/skip", epID),
			map[string]any{"kind": "intro", "start": 5, "end": 90})
		guest.mustStatus(401, status, body, "guest cannot set marks")
		status, body = admin.do("POST", fmt.Sprintf("/api/episodes/%d/skip", epID),
			map[string]any{"kind": "intro", "start": 5, "end": 90})
		admin.mustStatus(201, status, body, "manual intro override")
		status, body = admin.do("POST", fmt.Sprintf("/api/episodes/%d/skip", epID),
			map[string]any{"kind": "intro", "start": 90, "end": 5})
		admin.mustStatus(400, status, body, "inverted range rejected")

		status, body = guest.do("GET", fmt.Sprintf("/api/episodes/%d/skip", epID), nil)
		guest.mustStatus(200, status, body, "skip after override")
		if d := data(body)["intro"].(map[string]any); d["start"] != float64(5) || d["end"] != float64(90) {
			t.Errorf("intro after manual override = %v", d)
		}

		// deleting a mark doesn't resurrect it from AniSkip on the next play
		status, body = admin.do("DELETE", fmt.Sprintf("/api/episodes/%d/skip/intro", epID), nil)
		admin.mustStatus(200, status, body, "delete intro mark")
		status, body = guest.do("GET", fmt.Sprintf("/api/episodes/%d/skip", epID), nil)
		guest.mustStatus(200, status, body, "skip after delete")
		if d := data(body); d["intro"] != nil || d["outro"] == nil {
			t.Errorf("after delete: %v, want intro gone, outro kept", d)
		}
	})

	t.Run("PlaybackProgress", func(t *testing.T) {
		srv2, fx2 := srv, fx
		c := &client{t: t, base: srv2.URL, ip: "10.1.0.16"}
		c.signup()

		// nothing saved yet
		status, body := c.do("GET", fmt.Sprintf("/api/episodes/%d/progress", fx2.episodeID), nil)
		c.mustStatus(200, status, body, "empty progress")
		if body["data"] != nil {
			t.Errorf("progress = %v, want null", body["data"])
		}

		// mid-episode save: position stored, not watched
		status, body = c.do("PUT", fmt.Sprintf("/api/episodes/%d/progress", fx2.episodeID),
			map[string]any{"position": 300.5, "duration": 1420.0})
		c.mustStatus(200, status, body, "save mid progress")
		if d := data(body); d["watched"] != false {
			t.Errorf("mid-episode save = %v, want watched=false", d)
		}
		status, body = c.do("GET", fmt.Sprintf("/api/episodes/%d/progress", fx2.episodeID), nil)
		c.mustStatus(200, status, body, "read progress back")
		if d := data(body); d["position"] != 300.5 {
			t.Errorf("resume position = %v", d)
		}

		// crossing 90% marks the episode watched → watchlist entry + history
		status, body = c.do("PUT", fmt.Sprintf("/api/episodes/%d/progress", fx2.episodeID),
			map[string]any{"position": 1400.0, "duration": 1420.0})
		c.mustStatus(200, status, body, "save near-end progress")
		if d := data(body); d["watched"] != true {
			t.Errorf("near-end save = %v, want watched=true", d)
		}
		status, body = c.do("GET", fmt.Sprintf("/api/users/me/watchlist/%d", fx2.animeID), nil)
		c.mustStatus(200, status, body, "auto-created watchlist entry")
		if d := data(body); d["episodesWatched"] != float64(1) || d["status"] != "watching" {
			t.Errorf("watchlist after auto-mark = %v", d)
		}
		status, body = c.do("GET", "/api/users/me/history?days=7", nil)
		c.mustStatus(200, status, body, "history after auto-mark")
		if days := body["data"].([]any); len(days) == 0 {
			t.Error("watch_history got no delta from auto-mark")
		}

		// replaying the same episode doesn't double-count
		status, body = c.do("PUT", fmt.Sprintf("/api/episodes/%d/progress", fx2.episodeID),
			map[string]any{"position": 1410.0, "duration": 1420.0})
		c.mustStatus(200, status, body, "second near-end save")
		if d := data(body); d["watched"] != false {
			t.Errorf("rewatch save = %v, want watched=false (no double count)", d)
		}

		// a 'completed' entry is never demoted by a rewatch
		status, body = c.do("POST", "/api/users/me/watchlist", map[string]any{
			"animeId": fx2.animeID, "status": "completed"})
		c.mustStatus(201, status, body, "set completed")
		status, body = c.do("PUT", fmt.Sprintf("/api/episodes/%d/progress", fx2.episodeID),
			map[string]any{"position": 1400.0, "duration": 1420.0})
		c.mustStatus(200, status, body, "rewatch after completed")
		status, body = c.do("GET", fmt.Sprintf("/api/users/me/watchlist/%d", fx2.animeID), nil)
		c.mustStatus(200, status, body, "watchlist unchanged")
		if d := data(body); d["status"] != "completed" {
			t.Errorf("status after rewatch = %v, want completed", d)
		}

		// guests can't write progress
		guest := &client{t: t, base: srv2.URL, ip: "10.1.0.17"}
		status, body = guest.do("PUT", fmt.Sprintf("/api/episodes/%d/progress", fx2.episodeID),
			map[string]any{"position": 10.0})
		guest.mustStatus(401, status, body, "guest progress rejected")
	})

	t.Run("PublicProfileAndLists", func(t *testing.T) {
		c := &client{t: t, base: srv.URL, ip: "10.1.0.8"}
		username := c.signup()
		status, body := c.do("POST", "/api/users/me/watchlist", map[string]any{
			"animeId": fx.animeID, "status": "plan-to-watch"})
		c.mustStatus(201, status, body, "seed watchlist")

		anon := &client{t: t, base: srv.URL, ip: "10.1.0.9"}
		status, body = anon.do("GET", "/api/users/"+username, nil)
		anon.mustStatus(200, status, body, "public profile")
		user := body["user"].(map[string]any)
		if _, leaked := user["email"]; leaked {
			t.Fatal("public profile leaks email")
		}
		if body["network"] == nil || body["stats"] == nil {
			t.Errorf("profile missing network/stats: %v", body)
		}

		status, body = anon.do("GET", "/api/users/"+username+"/watchlist?status=plan-to-watch", nil)
		anon.mustStatus(200, status, body, "public watchlist")
		if entries := body["data"].([]any); len(entries) != 1 {
			t.Errorf("public watchlist entries = %d, want 1", len(entries))
		}

		status, body = anon.do("GET", "/api/users/no_such_user_404", nil)
		anon.mustStatus(404, status, body, "unknown user")
	})

	t.Run("CatalogPaginationShape", func(t *testing.T) {
		c := &client{t: t, base: srv.URL, ip: "10.1.0.10"}
		status, body := c.do("GET", "/api/anime?limit=1&sort=title", nil)
		c.mustStatus(200, status, body, "list anime")
		p := body["pagination"].(map[string]any)
		if p["limit"] != float64(1) || p["total"].(float64) < 2 || p["totalPages"].(float64) < 2 {
			t.Errorf("pagination = %v", p)
		}
		if rows := body["data"].([]any); len(rows) != 1 {
			t.Errorf("rows = %d, want 1", len(rows))
		}

		status, body = c.do("GET", "/api/anime?genres=Action&limit=5", nil)
		c.mustStatus(200, status, body, "genre filter (500'd in the TS backend)")
		rows := body["data"].([]any)
		if len(rows) != 1 || rows[0].(map[string]any)["title"] != "Test Show" {
			t.Errorf("genre filter rows = %v", rows)
		}

		status, body = c.do("GET", "/api/anime/999999", nil)
		c.mustStatus(404, status, body, "unknown anime id")
		status, body = c.do("GET", "/api/definitely/not/here", nil)
		c.mustStatus(404, status, body, "router 404")
		if body["error"] != "Not Found" {
			t.Errorf("404 shape = %v", body)
		}
	})

	t.Run("AvatarMagicBytes", func(t *testing.T) {
		c := &client{t: t, base: srv.URL, ip: "10.1.0.11"}
		c.signup()

		png := append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, make([]byte, 64)...)
		status, body := c.upload("avatar.png", "image/png", png)
		c.mustStatus(200, status, body, "real png")
		if !strings.HasSuffix(body["avatarUrl"].(string), ".png") {
			t.Errorf("avatarUrl = %v", body["avatarUrl"])
		}

		// a text file wearing a .png name and a spoofed MIME type must be rejected
		status, body = c.upload("fake.png", "image/png", []byte("definitely not an image"))
		c.mustStatus(400, status, body, "fake png")
	})

	// The release pipeline: staging upload → editor autosave →
	// submit → verify gate, plus the bring-your-own-RO-sub shortcut.
	t.Run("Releases", func(t *testing.T) {
		tr := &client{t: t, base: srv.URL, ip: "10.1.0.30"}
		trName := tr.signupWithRole("translator")
		srt := "1\n00:00:01,000 --> 00:00:03,000\nHello.\n\n2\n00:00:04,000 --> 00:00:06,000\nGoodbye.\n"

		// plain users are below the role floor
		user := &client{t: t, base: srv.URL, ip: "10.1.0.31"}
		user.signup()
		status, body := user.postForm("/api/releases", map[string]string{"animeId": "1", "episodeNumber": "1"}, nil)
		user.mustStatus(403, status, body, "plain user creates release")

		// a release without any subtitle is unreviewable — rejected up front
		status, body = tr.postForm("/api/releases",
			map[string]string{"animeId": fmt.Sprint(fx.animeID), "episodeNumber": "2"},
			map[string][2]string{"video": {"ep2.mp4", "fake video bytes"}})
		tr.mustStatus(400, status, body, "release without sub")

		status, body = tr.postForm("/api/releases",
			map[string]string{"animeId": fmt.Sprint(fx.animeID), "episodeNumber": "2"},
			map[string][2]string{
				"video": {"ep2.mp4", "fake video bytes"},
				"sub":   {"ep2.en.srt", srt},
			})
		tr.mustStatus(201, status, body, "create release")
		rel := data(body)
		relID := int(rel["id"].(float64))
		if rel["state"] != "draft" || rel["hasVideo"] != true {
			t.Errorf("fresh release = %v", rel)
		}

		relPath := fmt.Sprintf("/api/releases/%d", relID)
		status, body = tr.do("GET", relPath+"/events", nil)
		tr.mustStatus(200, status, body, "list events")
		events := body["data"].([]any)
		if len(events) != 2 {
			t.Fatalf("parsed %d events, want 2", len(events))
		}
		ev0 := events[0].(map[string]any)
		if ev0["enText"] != "Hello." || ev0["roText"] != "" || ev0["startMs"] != float64(1000) {
			t.Errorf("event 0 = %v", ev0)
		}

		// an untranslated draft can't be submitted
		status, body = tr.do("POST", relPath+"/submit", nil)
		tr.mustStatus(400, status, body, "submit empty draft")

		// editor autosave, then the live-preview VTT carries only translated rows
		status, body = tr.do("PUT", relPath+"/events/0", map[string]any{"roText": "Salut."})
		tr.mustStatus(200, status, body, "autosave row")
		status, vtt := tr.getRaw(relPath + "/draft.vtt")
		if status != 200 || !strings.Contains(vtt, "Salut.") || strings.Contains(vtt, "Goodbye") {
			t.Errorf("draft.vtt (status %d):\n%s", status, vtt)
		}

		// the staging video streams back (native preview src)
		status, vid := tr.getRaw(relPath + "/video")
		if status != 200 || vid != "fake video bytes" {
			t.Errorf("staging video: status %d, body %q", status, vid)
		}

		// other users see nothing
		status, body = user.do("GET", relPath, nil)
		user.mustStatus(403, status, body, "stranger reads release")

		status, body = tr.do("POST", relPath+"/submit", nil)
		tr.mustStatus(200, status, body, "submit")

		// translators are below the verify gate; verifiers are not
		status, body = tr.do("POST", relPath+"/approve", nil)
		tr.mustStatus(403, status, body, "translator approves own work")

		ver := &client{t: t, base: srv.URL, ip: "10.1.0.32"}
		verName := ver.signupWithRole("verifier")
		status, body = ver.do("GET", "/api/releases?all=1&state=in_review", nil)
		ver.mustStatus(200, status, body, "verify queue")
		queue := body["data"].([]any)
		if len(queue) != 1 || int(queue[0].(map[string]any)["id"].(float64)) != relID {
			t.Errorf("verify queue = %v", queue)
		}

		status, body = ver.do("POST", relPath+"/request-changes", map[string]any{})
		ver.mustStatus(400, status, body, "request changes without notes")
		status, body = ver.do("POST", relPath+"/request-changes", map[string]any{"notes": "Linia 2 e netradusă"})
		ver.mustStatus(200, status, body, "request changes")

		status, body = tr.do("GET", relPath, nil)
		tr.mustStatus(200, status, body, "read after changes requested")
		if d := data(body); d["state"] != "changes_requested" || d["reviewNotes"] != "Linia 2 e netradusă" {
			t.Errorf("after request-changes = %v", d)
		}

		// fix, resubmit, approve; a second approve must hit the stale guard
		status, body = tr.do("PUT", relPath+"/events/1", map[string]any{"roText": "Pa."})
		tr.mustStatus(200, status, body, "fix row")
		status, body = tr.do("POST", relPath+"/submit", nil)
		tr.mustStatus(200, status, body, "resubmit")
		status, body = ver.do("POST", relPath+"/approve", nil)
		ver.mustStatus(200, status, body, "approve")
		status, body = ver.do("POST", relPath+"/approve", nil)
		ver.mustStatus(404, status, body, "double approve")

		// approval is only the quality verdict — the release waits for the
		// coordinator's publish step
		status, body = ver.do("GET", relPath, nil)
		ver.mustStatus(200, status, body, "read approved release")
		if d := data(body); d["state"] != "approved" {
			t.Errorf("state after approve = %v, want approved", d["state"])
		}
		status, body = ver.do("POST", relPath+"/publish", nil)
		ver.mustStatus(403, status, body, "verifier publishes")
		status, body = tr.do("POST", relPath+"/publish", nil)
		tr.mustStatus(403, status, body, "translator publishes")

		co := &client{t: t, base: srv.URL, ip: "10.1.0.33"}
		coName := co.signupWithRole("coordinator")

		// mal-search + single-title import are open to translators now (they pull
		// a missing series into the catalog from the new-release form); the
		// bulk/manual catalog tools stay coordinator-only
		status, body = tr.do("POST", "/api/anime/", map[string]any{"title": "Translator Cannot Create"})
		tr.mustStatus(403, status, body, "translator manual create stays forbidden")

		// an anime episode with no source is unwatchable — publishing is blocked
		status, body = co.do("POST", relPath+"/publish", nil)
		co.mustStatus(400, status, body, "publish anime without a source")

		status, body = co.do("POST", relPath+"/publish", map[string]any{
			"sources": []map[string]any{
				{"hostingUrl": "https://cdn.example/rel-ep2"},
				{"hostingUrl": "https://mirror.example/rel-ep2"},
			},
		})
		co.mustStatus(200, status, body, "coordinator publishes")
		pubEpID := int(body["episodeId"].(float64))
		status, body = co.do("POST", relPath+"/publish", nil)
		co.mustStatus(409, status, body, "double publish")

		// publish created the episode with the source attached and put the
		// RO track live, served from /uploads
		status, body = ver.do("GET", relPath, nil)
		ver.mustStatus(200, status, body, "read published release")
		if d := data(body); d["state"] != "published" {
			t.Errorf("state after publish = %v, want published", d["state"])
		}
		// Credits name all three roles. The coordinator is the one
		// the pipeline used to lose: uploader and reviewer were already on the
		// row, but who pressed publish left no trace at all.
		status, body = tr.do("GET", fmt.Sprintf("/api/anime/%d/episodes/2/credits", fx.animeID), nil)
		tr.mustStatus(200, status, body, "episode credits")
		cr := data(body)
		for role, want := range map[string]string{
			"translator":  trName,
			"verifier":    verName,
			"coordinator": coName,
		} {
			who, _ := cr[role].(map[string]any)
			if who == nil {
				t.Errorf("credits.%s missing — the byline would drop that role", role)
				continue
			}
			if got, _ := who["username"].(string); got != want {
				t.Errorf("credits.%s = %q, want %q", role, got, want)
			}
		}

		status, body = tr.do("GET", fmt.Sprintf("/api/admin/episodes/%d/links", pubEpID), nil)
		tr.mustStatus(200, status, body, "published episode links")
		if links := body["data"].([]any); len(links) != 2 ||
			links[0].(map[string]any)["hostingUrl"] != "https://cdn.example/rel-ep2" {
			t.Errorf("published episode links = %v", body["data"])
		}
		// Publishing writes every language the release actually has text for.
		// This one was uploaded with both, so both go live: the RO track is
		// the product, the EN one is what you read it against later.
		status, body = ver.do("GET", fmt.Sprintf("/api/episodes/%d/subtitles", pubEpID), nil)
		ver.mustStatus(200, status, body, "published subtitles")
		byLang := map[string]string{}
		for _, s := range body["data"].([]any) {
			m := s.(map[string]any)
			lang, _ := m["language"].(string)
			url, _ := m["url"].(string)
			if !strings.HasPrefix(url, "/uploads/subs/") {
				t.Errorf("published subtitle %q not served from uploads: %v", lang, m)
			}
			byLang[lang] = url
		}
		if _, ok := byLang["ro"]; !ok {
			t.Fatalf("published subtitles = %v, want a RO track", body["data"])
		}
		status, vttBody := ver.getRaw(byLang["ro"])
		if status != 200 || !strings.Contains(vttBody, "WEBVTT") ||
			!strings.Contains(vttBody, "Salut.") || !strings.Contains(vttBody, "Pa.") {
			t.Errorf("published RO vtt (status %d):\n%s", status, vttBody)
		}
		if enURL, ok := byLang["en"]; ok {
			status, vttBody = ver.getRaw(enURL)
			if status != 200 || !strings.Contains(vttBody, "WEBVTT") ||
				!strings.Contains(vttBody, "Hello.") {
				t.Errorf("published EN vtt (status %d):\n%s", status, vttBody)
			}
		}

		// a release for a series the catalog doesn't have yet: the translator
		// proposes a title; the coordinator must link a real series to publish
		roSrtGhost := "1\n00:00:01,000 --> 00:00:03,000\nSerie nouă!\n"
		status, body = tr.postForm("/api/releases",
			map[string]string{"proposedTitle": "Serie Care Nu Există", "episodeNumber": "7"},
			map[string][2]string{
				"video": {"ghost.mp4", "ghost video"},
				"roSub": {"ghost.ro.srt", roSrtGhost},
			})
		tr.mustStatus(201, status, body, "create proposed-title release")
		ghost := data(body)
		if ghost["proposedTitle"] != "Serie Care Nu Există" || ghost["animeId"] != nil {
			t.Errorf("proposed-title release = %v", ghost)
		}
		ghostPath := fmt.Sprintf("/api/releases/%d", int(ghost["id"].(float64)))
		status, body = ver.do("POST", ghostPath+"/approve", nil)
		ver.mustStatus(200, status, body, "approve proposed-title release")
		status, body = co.do("POST", ghostPath+"/publish", nil)
		co.mustStatus(400, status, body, "publish without a series")
		status, body = co.do("POST", ghostPath+"/publish", map[string]any{
			"animeId": fx.animeID, "episodeNumber": 8,
			"sources": []map[string]any{{"hostingUrl": "https://cdn.example/ghost-ep8"}},
		})
		co.mustStatus(200, status, body, "publish with coordinator mapping")
		status, body = co.do("GET", ghostPath, nil)
		if d := data(body); d["state"] != "published" ||
			int(d["animeId"].(float64)) != fx.animeID || d["episodeNumber"] != float64(8) ||
			d["proposedTitle"] != nil {
			t.Errorf("published ghost release = %v", data(body))
		}
		status, body = co.do("DELETE", ghostPath, nil)
		co.mustStatus(200, status, body, "coordinator cleans up ghost release")

		// approved work is out of the uploader's hands; verifier-class can delete
		status, body = tr.do("DELETE", relPath, nil)
		tr.mustStatus(403, status, body, "uploader deletes approved release")
		status, body = ver.do("DELETE", relPath, nil)
		ver.mustStatus(200, status, body, "verifier deletes release")

		// bring-your-own-sub (the Aegisub path): finished RO sub skips the
		// editor — rows arrive translated+edited, state lands in review
		roSrt := "1\n00:00:01,000 --> 00:00:03,000\nSalut!\n\n2\n00:00:04,000 --> 00:00:06,000\nPa!\n"
		status, body = tr.postForm("/api/releases",
			map[string]string{"animeId": fmt.Sprint(fx.animeID), "episodeNumber": "3"},
			map[string][2]string{
				"video": {"ep3.mkv", "more fake video"},
				"sub":   {"ep3.en.srt", srt},
				"roSub": {"ep3.ro.srt", roSrt},
			})
		tr.mustStatus(201, status, body, "bring-your-own-sub release")
		byo := data(body)
		if byo["state"] != "in_review" {
			t.Errorf("byo state = %v, want in_review", byo["state"])
		}
		status, body = tr.do("GET", fmt.Sprintf("/api/releases/%v/events", byo["id"]), nil)
		tr.mustStatus(200, status, body, "byo events")
		bev := body["data"].([]any)[0].(map[string]any)
		if bev["roText"] != "Salut!" || bev["enText"] != "Hello." || bev["edited"] != true {
			t.Errorf("byo event 0 = %v", bev)
		}
	})

	// Custom user lists ("Liste"): owner CRUD, mixed anime/manga items,
	// public-vs-private visibility.
	t.Run("UserLists", func(t *testing.T) {
		owner := &client{t: t, base: srv.URL, ip: "10.1.0.95"}
		owner.signup()
		other := &client{t: t, base: srv.URL, ip: "10.1.0.96"}
		other.signup()
		guest := &client{t: t, base: srv.URL, ip: "10.1.0.97"}

		status, body := guest.do("POST", "/api/lists", map[string]any{"title": "X"})
		guest.mustStatus(401, status, body, "guest creates list")
		status, body = owner.do("POST", "/api/lists", map[string]any{"title": "   "})
		owner.mustStatus(400, status, body, "blank title")

		status, body = owner.do("POST", "/api/lists", map[string]any{
			"title": "Top emoții", "description": "Cele care dor.",
		})
		owner.mustStatus(201, status, body, "create public list")
		pub := data(body)
		pubID := int(pub["id"].(float64))
		if pub["isPublic"] != true || pub["itemCount"] != float64(0) {
			t.Errorf("fresh list = %v", pub)
		}

		status, body = owner.do("POST", "/api/lists", map[string]any{
			"title": "Doar pentru mine", "isPublic": false,
		})
		owner.mustStatus(201, status, body, "create private list")
		privID := int(data(body)["id"].(float64))

		// items: one anime, one manga; duplicates conflict
		status, body = owner.do("POST", fmt.Sprintf("/api/lists/%d/items", pubID),
			map[string]any{"mediaType": "anime", "mediaId": fx.animeID, "note": "începe aici"})
		owner.mustStatus(201, status, body, "add anime item")
		itemID := int(data(body)["id"].(float64))
		status, body = owner.do("POST", fmt.Sprintf("/api/lists/%d/items", pubID),
			map[string]any{"mediaType": "manga", "mediaId": fx.mangaID})
		owner.mustStatus(201, status, body, "add manga item")
		status, body = owner.do("POST", fmt.Sprintf("/api/lists/%d/items", pubID),
			map[string]any{"mediaType": "anime", "mediaId": fx.animeID})
		owner.mustStatus(409, status, body, "duplicate item")
		status, body = owner.do("POST", fmt.Sprintf("/api/lists/%d/items", pubID),
			map[string]any{"mediaType": "anime", "mediaId": 999999})
		owner.mustStatus(404, status, body, "missing title")

		// strangers can't touch someone else's list
		status, body = other.do("POST", fmt.Sprintf("/api/lists/%d/items", pubID),
			map[string]any{"mediaType": "anime", "mediaId": fx.anime2ID})
		other.mustStatus(403, status, body, "stranger adds item")

		// visibility: guests read public lists with items; private ones 404
		status, body = guest.do("GET", fmt.Sprintf("/api/lists/%d", pubID), nil)
		guest.mustStatus(200, status, body, "guest reads public list")
		items := body["items"].([]any)
		if len(items) != 2 || items[0].(map[string]any)["note"] != "începe aici" ||
			items[1].(map[string]any)["mangaId"] == nil {
			t.Errorf("public list items = %v", items)
		}
		status, body = guest.do("GET", fmt.Sprintf("/api/lists/%d", privID), nil)
		guest.mustStatus(404, status, body, "guest reads private list")
		status, body = owner.do("GET", fmt.Sprintf("/api/lists/%d", privID), nil)
		owner.mustStatus(200, status, body, "owner reads private list")

		// mine + the public browse feed (content-bearing lists only)
		status, body = owner.do("GET", "/api/lists/mine", nil)
		owner.mustStatus(200, status, body, "my lists")
		if mine := body["data"].([]any); len(mine) != 2 {
			t.Errorf("my lists = %d, want 2", len(mine))
		}
		status, body = guest.do("GET", "/api/lists", nil)
		guest.mustStatus(200, status, body, "public feed")
		feed := body["data"].([]any)
		found := false
		for _, l := range feed {
			m := l.(map[string]any)
			if int(m["id"].(float64)) == pubID {
				found = true
				// covers stays empty here — the fixture titles carry no posters
				if m["itemCount"] != float64(2) {
					t.Errorf("feed entry = %v", m)
				}
			}
			if int(m["id"].(float64)) == privID {
				t.Errorf("private list leaked into the public feed")
			}
		}
		if !found {
			t.Errorf("public list missing from feed")
		}

		// note edit, item removal, list update, delete
		status, body = owner.do("PUT", fmt.Sprintf("/api/lists/%d/items/%d", pubID, itemID),
			map[string]any{"note": "de fapt începe aici"})
		owner.mustStatus(200, status, body, "edit note")
		status, body = owner.do("DELETE", fmt.Sprintf("/api/lists/%d/items/%d", pubID, itemID), nil)
		owner.mustStatus(200, status, body, "remove item")
		status, body = owner.do("PUT", fmt.Sprintf("/api/lists/%d", pubID),
			map[string]any{"title": "Top emoții v2", "isPublic": false})
		owner.mustStatus(200, status, body, "update list")
		status, body = guest.do("GET", fmt.Sprintf("/api/lists/%d", pubID), nil)
		guest.mustStatus(404, status, body, "now-private list hidden")
		status, body = other.do("DELETE", fmt.Sprintf("/api/lists/%d", pubID), nil)
		other.mustStatus(403, status, body, "stranger deletes list")
		status, body = owner.do("DELETE", fmt.Sprintf("/api/lists/%d", pubID), nil)
		owner.mustStatus(200, status, body, "delete list")
		status, body = owner.do("GET", fmt.Sprintf("/api/lists/%d", pubID), nil)
		owner.mustStatus(404, status, body, "deleted list gone")
	})

	// The manga release variant: bring-your-own-pages, straight to
	// review; verify is a page flipper; publish writes chapter_pages.
	t.Run("MangaReleases", func(t *testing.T) {
		tr := &client{t: t, base: srv.URL, ip: "10.1.0.90"}
		tr.signupWithRole("translator")
		user := &client{t: t, base: srv.URL, ip: "10.1.0.91"}
		user.signup()

		pngBytes := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{1}, 64)...)
		png2 := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{2}, 64)...)
		// multipart poster allowing several files under one field
		post := func(c *client, path string, fields map[string]string, files map[string]map[string][]byte) (int, map[string]any) {
			t.Helper()
			var buf bytes.Buffer
			mw := multipart.NewWriter(&buf)
			for k, v := range fields {
				_ = mw.WriteField(k, v)
			}
			for field, byName := range files {
				names := make([]string, 0, len(byName))
				for n := range byName {
					names = append(names, n)
				}
				sort.Sort(sort.Reverse(sort.StringSlice(names))) // send out of order
				for _, n := range names {
					fw, _ := mw.CreateFormFile(field, n)
					fw.Write(byName[n]) //nolint:errcheck
				}
			}
			mw.Close()
			req, _ := http.NewRequest("POST", srv.URL+path, &buf)
			req.Header.Set("Content-Type", mw.FormDataContentType())
			req.Header.Set("X-Forwarded-For", c.ip)
			if c.token != "" {
				req.Header.Set("Authorization", "Bearer "+c.token)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			var out map[string]any
			_ = json.NewDecoder(resp.Body).Decode(&out)
			return resp.StatusCode, out
		}

		// pages are mandatory; non-images are sniffed out
		status, body := post(tr, "/api/releases",
			map[string]string{"medium": "manga", "mangaId": fmt.Sprint(fx.mangaID), "chapterNumber": "10.5"}, nil)
		tr.mustStatus(400, status, body, "manga release without pages")
		status, body = post(tr, "/api/releases",
			map[string]string{"medium": "manga", "mangaId": fmt.Sprint(fx.mangaID), "chapterNumber": "10.5"},
			map[string]map[string][]byte{"pages": {"01.png": []byte("#!/bin/sh\n")}})
		tr.mustStatus(400, status, body, "manga release with non-image page")

		// RO pages + EN originals → lands straight in review with counts
		status, body = post(tr, "/api/releases",
			map[string]string{"medium": "manga", "mangaId": fmt.Sprint(fx.mangaID), "chapterNumber": "10.5"},
			map[string]map[string][]byte{
				"pages":   {"01.png": pngBytes, "02.png": png2},
				"enPages": {"raw01.png": pngBytes},
			})
		tr.mustStatus(201, status, body, "create manga release")
		rel := data(body)
		relID := int(rel["id"].(float64))
		if rel["medium"] != "manga" || rel["state"] != "in_review" ||
			rel["pageCount"] != float64(2) || rel["enPageCount"] != float64(1) ||
			rel["chapterNumber"] != float64(10.5) {
			t.Errorf("fresh manga release = %v", rel)
		}
		relPath := fmt.Sprintf("/api/releases/%d", relID)

		// the verify flipper reads staged pages; 1-based, sorted by filename
		status, page := tr.getRaw(relPath + "/page/ro/1")
		if status != 200 || page != string(pngBytes) {
			t.Errorf("staged page 1: status %d, %d bytes", status, len(page))
		}
		status, _ = tr.getRaw(relPath + "/page/ro/3")
		if status != 404 {
			t.Errorf("staged page 3 status = %d, want 404", status)
		}
		status, body = user.do("GET", relPath, nil)
		user.mustStatus(403, status, body, "stranger reads manga release")

		// request-changes → uploader replaces the RO edition → resubmit
		ver := &client{t: t, base: srv.URL, ip: "10.1.0.92"}
		ver.signupWithRole("verifier")
		status, body = ver.do("POST", relPath+"/request-changes", map[string]any{"notes": "Pagina 2 are typo"})
		ver.mustStatus(200, status, body, "request changes")
		status, body = post(tr, relPath+"/pages", map[string]string{"lang": "ro"},
			map[string]map[string][]byte{"pages": {"01.png": pngBytes, "02.png": png2, "03.png": pngBytes}})
		tr.mustStatus(200, status, body, "re-upload pages")
		if body["count"] != float64(3) {
			t.Errorf("re-upload response = %v", body)
		}
		status, body = tr.do("POST", relPath+"/submit", nil)
		tr.mustStatus(200, status, body, "resubmit manga release")
		status, body = ver.do("POST", relPath+"/approve", nil)
		ver.mustStatus(200, status, body, "approve manga release")

		// publish: coordinator-only; creates the chapter and its own pages
		status, body = tr.do("POST", relPath+"/publish", nil)
		tr.mustStatus(403, status, body, "translator publishes manga")
		co := &client{t: t, base: srv.URL, ip: "10.1.0.93"}
		co.signupWithRole("coordinator")
		status, body = co.do("POST", relPath+"/publish", nil)
		co.mustStatus(200, status, body, "coordinator publishes manga")
		chID := int(body["chapterId"].(float64))
		status, body = co.do("POST", relPath+"/publish", nil)
		co.mustStatus(409, status, body, "double publish manga")

		status, body = user.do("GET", fmt.Sprintf("/api/chapters/%d/pages", chID), nil)
		user.mustStatus(200, status, body, "read published chapter pages")
		d := data(body)
		langs := d["languages"].([]any)
		if d["language"] != "ro" || len(d["pages"].([]any)) != 3 || len(langs) != 2 {
			t.Fatalf("published chapter pages = %v", d)
		}
		resp, err := http.Get(srv.URL + d["pages"].([]any)[1].(string))
		if err != nil {
			t.Fatal(err)
		}
		served, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || !bytes.Equal(served, png2) {
			t.Errorf("serving published page: status %d, %d bytes", resp.StatusCode, len(served))
		}

		// publish freed the staging pages; the release stays readable
		status, _ = tr.getRaw(relPath + "/page/ro/1")
		if status != 404 {
			t.Errorf("staged page after publish status = %d, want 404", status)
		}
		status, body = tr.do("GET", relPath, nil)
		tr.mustStatus(200, status, body, "read published manga release")
		if d := data(body); d["state"] != "published" {
			t.Errorf("published manga release = %v", d)
		}

		// proposed-title path: publish demands a catalog series mapping
		status, body = post(tr, "/api/releases",
			map[string]string{"medium": "manga", "proposedTitle": "Manga Fantomă", "chapterNumber": "1"},
			map[string]map[string][]byte{"pages": {"01.png": pngBytes}})
		tr.mustStatus(201, status, body, "proposed-title manga release")
		ghostPath := fmt.Sprintf("/api/releases/%d", int(data(body)["id"].(float64)))
		status, body = ver.do("POST", ghostPath+"/approve", nil)
		ver.mustStatus(200, status, body, "approve ghost manga")
		status, body = co.do("POST", ghostPath+"/publish", nil)
		co.mustStatus(400, status, body, "publish ghost manga without series")
		status, body = co.do("POST", ghostPath+"/publish", map[string]any{
			"mangaId": fx.mangaID, "chapterNumber": 11,
		})
		co.mustStatus(200, status, body, "publish ghost manga with mapping")
		status, body = co.do("GET", ghostPath, nil)
		if d := data(body); d["state"] != "published" ||
			int(d["mangaId"].(float64)) != fx.mangaID || d["chapterNumber"] != float64(11) ||
			d["proposedTitle"] != nil {
			t.Errorf("published ghost manga release = %v", data(body))
		}
	})

	// Auto-translate: fills only empty RO rows, never human edits.
	t.Run("AutoTranslate", func(t *testing.T) {
		tr := &client{t: t, base: srv.URL, ip: "10.1.0.40"}
		tr.signupWithRole("translator")

		srt := "1\n00:00:01,000 --> 00:00:03,000\nHello there.\n\n2\n00:00:04,000 --> 00:00:06,000\nGeneral Kenobi.\n"
		status, body := tr.postForm("/api/releases",
			map[string]string{"animeId": fmt.Sprint(fx.animeID), "episodeNumber": "9"},
			map[string][2]string{
				"video": {"ep9.mp4", "fake"},
				"sub":   {"ep9.en.srt", srt},
			})
		tr.mustStatus(201, status, body, "create release")
		relID := int(data(body)["id"].(float64))
		relPath := fmt.Sprintf("/api/releases/%d", relID)

		// human translates row 1 before the machine runs
		status, body = tr.do("PUT", relPath+"/events/1", map[string]any{"roText": "Generale Kenobi."})
		tr.mustStatus(200, status, body, "human edit")

		status, body = tr.do("POST", relPath+"/translate", nil)
		tr.mustStatus(202, status, body, "start auto-translate")
		if body["pending"] != float64(1) {
			t.Errorf("pending = %v, want 1 (only the untranslated row)", body["pending"])
		}

		// the run is async — poll until row 0 is filled
		var events []any
		deadline := time.Now().Add(10 * time.Second)
		for {
			status, body = tr.do("GET", relPath+"/events", nil)
			tr.mustStatus(200, status, body, "poll events")
			events = body["data"].([]any)
			if events[0].(map[string]any)["roText"] != "" || time.Now().After(deadline) {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		ev0 := events[0].(map[string]any)
		ev1 := events[1].(map[string]any)
		if ev0["roText"] != "RO: Hello there." || ev0["edited"] != false {
			t.Errorf("machine row = %v (want machine text, edited=false)", ev0)
		}
		if ev1["roText"] != "Generale Kenobi." || ev1["edited"] != true {
			t.Errorf("human row = %v (must be untouched by the machine)", ev1)
		}
	})

	t.Run("AdminPanel", func(t *testing.T) {
		admin := &client{t: t, base: srv.URL, ip: "10.1.0.40"}
		admin.signupWithRole("admin")
		user := &client{t: t, base: srv.URL, ip: "10.1.0.41"}
		user.signup()

		status, body := user.do("GET", "/api/admin/health-report", nil)
		user.mustStatus(403, status, body, "plain user blocked from admin")

		// test-source: validation results come back as data.ok, not
		// HTTP errors — a dead source is a result
		status, body = admin.do("POST", "/api/admin/test-source", map[string]any{
			"kind": "embed", "hostingUrl": "https://player.example.com/e/abc"})
		admin.mustStatus(200, status, body, "test valid embed")
		if data(body)["ok"] != true {
			t.Errorf("valid embed test = %v, want ok", data(body))
		}
		status, body = admin.do("POST", "/api/admin/test-source", map[string]any{
			"kind": "embed", "hostingUrl": "http://player.example.com/e/abc"})
		admin.mustStatus(200, status, body, "test http embed")
		if data(body)["ok"] != false {
			t.Errorf("http embed test = %v, want not ok", data(body))
		}
		status, body = admin.do("POST", "/api/admin/test-source", map[string]any{
			"kind": "extract", "provider": "direct", "providerRef": "http://cdn.example/x.m3u8"})
		admin.mustStatus(200, status, body, "test unresolvable extract")
		if data(body)["ok"] != false {
			t.Errorf("http extract test = %v, want not ok", data(body))
		}

		// an episode whose only source the health checker has flagged dead
		status, body = admin.do("POST", fmt.Sprintf("/api/anime/%d/episodes", fx.anime2ID),
			map[string]any{"episodeNumber": 42})
		admin.mustStatus(201, status, body, "create episode")
		epID := int(data(body)["id"].(float64))
		status, body = admin.do("POST", fmt.Sprintf("/api/episodes/%d/links", epID),
			map[string]any{"hostingUrl": "https://cdn.example/page", "kind": "extract",
				"provider": "direct", "providerRef": "https://cdn.example/dead.m3u8", "priority": 5})
		admin.mustStatus(201, status, body, "add extract source")
		linkID := int(data(body)["id"].(float64))

		// PUT /api/links/{id} patches without clobbering the rest
		status, body = user.do("PUT", fmt.Sprintf("/api/links/%d", linkID), map[string]any{"priority": 1})
		user.mustStatus(403, status, body, "plain user cannot patch links")
		status, body = admin.do("PUT", fmt.Sprintf("/api/links/%d", linkID), map[string]any{"priority": 42})
		admin.mustStatus(200, status, body, "patch link priority")
		if l := data(body); l["priority"] != float64(42) || l["isActive"] != true || l["kind"] != "extract" {
			t.Errorf("patched link = %v, want priority 42 with everything else untouched", l)
		}

		// what cmd/healthcheck writes on a failed probe
		ctx := context.Background()
		conn, err := pgx.Connect(ctx, os.Getenv("TEST_DATABASE_URL"))
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close(ctx)
		if _, err := conn.Exec(ctx,
			`UPDATE content_links SET last_ok = false, last_checked_at = now() WHERE id = $1`, linkID); err != nil {
			t.Fatal(err)
		}

		status, body = admin.do("GET", "/api/admin/health-report", nil)
		admin.mustStatus(200, status, body, "health report")
		rep := data(body)

		foundDead := false
		for _, d := range rep["deadSources"].([]any) {
			if int(d.(map[string]any)["id"].(float64)) == linkID {
				foundDead = true
			}
		}
		if !foundDead {
			t.Errorf("deadSources = %v, want link %d listed", rep["deadSources"], linkID)
		}

		missingSource := rep["missingSource"].(map[string]any)
		foundGap := false
		for _, e := range missingSource["episodes"].([]any) {
			if int(e.(map[string]any)["episodeId"].(float64)) == epID {
				foundGap = true
			}
		}
		if !foundGap {
			t.Errorf("missingSource = %v, want episode %d (its only source is dead)", missingSource, epID)
		}
		if total := rep["missingRoSub"].(map[string]any)["total"].(float64); total < 1 {
			t.Errorf("missingRoSub.total = %v, want at least the new episode", total)
		}
	})

	t.Run("Moderation", func(t *testing.T) {
		mod := &client{t: t, base: srv.URL, ip: "10.1.0.50"}
		modName := mod.signupWithRole("moderator")
		admin := &client{t: t, base: srv.URL, ip: "10.1.0.51"}
		admin.signupWithRole("admin")
		offender := &client{t: t, base: srv.URL, ip: "10.1.0.52"}
		offenderName := offender.signup()
		reporter := &client{t: t, base: srv.URL, ip: "10.1.0.53"}
		reporter.signup()

		// offender posts, reporter reports → queue shows it with context
		status, body := offender.do("POST", fmt.Sprintf("/api/anime/%d/comments", fx.animeID),
			map[string]any{"content": "spam spam spam"})
		offender.mustStatus(201, status, body, "post comment")
		commentID := int(data(body)["id"].(float64))
		status, body = reporter.do("POST", fmt.Sprintf("/api/comments/%d/report", commentID), nil)
		reporter.mustStatus(200, status, body, "report comment")

		status, body = reporter.do("GET", "/api/admin/reports", nil)
		reporter.mustStatus(403, status, body, "plain user blocked from reports")
		status, body = mod.do("GET", "/api/admin/reports", nil)
		mod.mustStatus(200, status, body, "moderator reads queue")
		reports := body["data"].([]any)
		found := false
		for _, rep := range reports {
			m := rep.(map[string]any)
			if int(m["id"].(float64)) == commentID {
				found = true
				if m["username"] != offenderName || m["contextTitle"] != "Test Show" {
					t.Errorf("report row = %v, want author + anime title context", m)
				}
			}
		}
		if !found {
			t.Fatalf("reports = %v, want comment %d queued", reports, commentID)
		}

		// dismiss clears the flag; the comment survives
		status, body = mod.do("POST", fmt.Sprintf("/api/admin/comments/%d/dismiss", commentID), nil)
		mod.mustStatus(200, status, body, "dismiss report")
		status, body = mod.do("POST", fmt.Sprintf("/api/admin/comments/%d/dismiss", commentID), nil)
		mod.mustStatus(404, status, body, "dismiss twice")

		// re-report, then moderator-delete (not the owner!)
		status, body = reporter.do("POST", fmt.Sprintf("/api/comments/%d/report", commentID), nil)
		reporter.mustStatus(200, status, body, "re-report")
		status, body = mod.do("DELETE", fmt.Sprintf("/api/admin/comments/%d", commentID), nil)
		mod.mustStatus(200, status, body, "moderator deletes someone else's comment")
		status, body = mod.do("GET", "/api/admin/reports", nil)
		mod.mustStatus(200, status, body, "queue after delete")
		if got := body["data"].([]any); len(got) != 0 {
			t.Errorf("queue = %v, want empty after delete", got)
		}

		// ban: blocks posting immediately and login afterwards
		var offenderID int
		status, body = mod.do("GET", "/api/admin/users?q="+offenderName, nil)
		mod.mustStatus(200, status, body, "find offender")
		for _, u := range body["data"].([]any) {
			if u.(map[string]any)["username"] == offenderName {
				offenderID = int(u.(map[string]any)["id"].(float64))
			}
		}
		if offenderID == 0 {
			t.Fatal("offender not found via user search")
		}
		status, body = mod.do("POST", fmt.Sprintf("/api/admin/users/%d/ban", offenderID),
			map[string]any{"banned": true})
		mod.mustStatus(200, status, body, "ban offender")
		status, body = offender.do("POST", fmt.Sprintf("/api/anime/%d/comments", fx.animeID),
			map[string]any{"content": "more spam"})
		offender.mustStatus(403, status, body, "banned user cannot post")
		status, body = offender.do("POST", "/api/auth/login", map[string]any{
			"email": offenderName + "@test.local", "password": "longpassword123"})
		offender.mustStatus(403, status, body, "banned user cannot log in")
		status, body = mod.do("POST", fmt.Sprintf("/api/admin/users/%d/ban", offenderID),
			map[string]any{"banned": false})
		mod.mustStatus(200, status, body, "unban")
		status, body = offender.do("POST", "/api/auth/login", map[string]any{
			"email": offenderName + "@test.local", "password": "longpassword123"})
		offender.mustStatus(200, status, body, "unbanned user logs in again")

		// role changes: admin-only, guarded against self-change and bad roles
		status, body = mod.do("PUT", fmt.Sprintf("/api/admin/users/%d/role", offenderID),
			map[string]any{"role": "translator"})
		mod.mustStatus(403, status, body, "moderator cannot change roles")
		status, body = admin.do("PUT", fmt.Sprintf("/api/admin/users/%d/role", offenderID),
			map[string]any{"role": "superuser"})
		admin.mustStatus(400, status, body, "invalid role rejected")
		status, body = admin.do("PUT", fmt.Sprintf("/api/admin/users/%d/role", offenderID),
			map[string]any{"role": "verifier"})
		admin.mustStatus(200, status, body, "admin promotes to verifier")

		// moderators can't be banned
		var modID int
		status, body = admin.do("GET", "/api/admin/users?q="+modName, nil)
		admin.mustStatus(200, status, body, "find moderator")
		for _, u := range body["data"].([]any) {
			if u.(map[string]any)["role"] == "moderator" {
				modID = int(u.(map[string]any)["id"].(float64))
			}
		}
		if modID != 0 {
			status, body = admin.do("POST", fmt.Sprintf("/api/admin/users/%d/ban", modID),
				map[string]any{"banned": true})
			admin.mustStatus(400, status, body, "moderators cannot be banned")
		}
	})

	t.Run("CatalogManagement", func(t *testing.T) {
		admin := &client{t: t, base: srv.URL, ip: "10.1.0.60"}
		admin.signupWithRole("admin")
		translator := &client{t: t, base: srv.URL, ip: "10.1.0.61"}
		translator.signupWithRole("translator")
		user := &client{t: t, base: srv.URL, ip: "10.1.0.62"}
		user.signup()

		// manual field edits — translator may, plain user may not
		status, body := user.do("PUT", fmt.Sprintf("/api/anime/%d", fx.anime2ID),
			map[string]any{"title": "nope"})
		user.mustStatus(403, status, body, "plain user cannot patch titles")
		status, body = translator.do("PUT", fmt.Sprintf("/api/anime/%d", fx.anime2ID),
			map[string]any{"title": "Edited Show", "year": 2001, "genres": []string{"Mystery"}})
		translator.mustStatus(200, status, body, "patch anime fields")
		got := data(body)
		if got["title"] != "Edited Show" || got["year"] != float64(2001) {
			t.Errorf("patched anime = %v", got)
		}
		if g := got["genres"].([]any); len(g) != 1 || g[0] != "Mystery" {
			t.Errorf("patched genres = %v", g)
		}
		// untouched fields survive the patch
		if got["status"] != "completed" {
			t.Errorf("status = %v, want untouched 'completed'", got["status"])
		}

		// delete cascades through episodes and their links (0009);
		// anime2 got episode 42 + an extract link in the AdminPanel subtest
		status, body = translator.do("DELETE", fmt.Sprintf("/api/anime/%d", fx.anime2ID), nil)
		translator.mustStatus(403, status, body, "translator cannot delete titles")
		status, body = admin.do("DELETE", fmt.Sprintf("/api/anime/%d", fx.anime2ID), nil)
		admin.mustStatus(200, status, body, "admin deletes title with episodes+links")
		status, body = admin.do("GET", fmt.Sprintf("/api/anime/%d", fx.anime2ID), nil)
		admin.mustStatus(404, status, body, "deleted title is gone")
	})

	t.Run("ChapterPages", func(t *testing.T) {
		admin := &client{t: t, base: srv.URL, ip: "10.1.0.70"}
		admin.signupWithRole("admin")
		guest := &client{t: t, base: srv.URL, ip: "10.1.0.71"}

		status, body := admin.do("POST", fmt.Sprintf("/api/manga/%d/chapters", fx.mangaID),
			map[string]any{"chapterNumber": 900})
		admin.mustStatus(201, status, body, "create chapter")
		chID := int(data(body)["id"].(float64))

		// no pages yet → empty payload, not an error (reader falls back to iframe)
		status, body = guest.do("GET", fmt.Sprintf("/api/chapters/%d/pages", chID), nil)
		guest.mustStatus(200, status, body, "pages of empty chapter")
		if d := data(body); len(d["languages"].([]any)) != 0 || len(d["pages"].([]any)) != 0 {
			t.Errorf("empty chapter pages = %v", d)
		}

		// guests can't write; RO set replaces atomically and syncs chapters.pages
		status, body = guest.do("PUT", fmt.Sprintf("/api/chapters/%d/pages", chID),
			map[string]any{"urls": []string{"https://cdn.example/p1.jpg"}})
		guest.mustStatus(401, status, body, "guest cannot set pages")
		status, body = admin.do("PUT", fmt.Sprintf("/api/chapters/%d/pages", chID),
			map[string]any{"language": "ro", "urls": []string{
				"https://cdn.example/ro/1.jpg", "https://cdn.example/ro/2.jpg", "https://cdn.example/ro/3.jpg"}})
		admin.mustStatus(200, status, body, "set ro pages")
		status, body = admin.do("PUT", fmt.Sprintf("/api/chapters/%d/pages", chID),
			map[string]any{"language": "en", "urls": []string{
				"https://cdn.example/en/1.jpg", "https://cdn.example/en/2.jpg"}})
		admin.mustStatus(200, status, body, "set en pages")
		status, body = admin.do("PUT", fmt.Sprintf("/api/chapters/%d/pages", chID),
			map[string]any{"language": "ro", "urls": []string{"http://insecure.example/1.jpg"}})
		admin.mustStatus(400, status, body, "http page URL rejected")

		// default is RO; explicit lang honored; unknown lang falls back to RO
		status, body = guest.do("GET", fmt.Sprintf("/api/chapters/%d/pages", chID), nil)
		guest.mustStatus(200, status, body, "read pages")
		d := data(body)
		if d["language"] != "ro" || len(d["pages"].([]any)) != 3 {
			t.Errorf("default pages = %v, want 3 RO pages", d)
		}
		if langs := d["languages"].([]any); len(langs) != 2 || langs[0] != "ro" {
			t.Errorf("languages = %v, want [ro en]", langs)
		}
		status, body = guest.do("GET", fmt.Sprintf("/api/chapters/%d/pages?lang=en", chID), nil)
		guest.mustStatus(200, status, body, "read en pages")
		if d := data(body); d["language"] != "en" || len(d["pages"].([]any)) != 2 {
			t.Errorf("en pages = %v", d)
		}
		status, body = guest.do("GET", fmt.Sprintf("/api/chapters/%d/pages?lang=jp", chID), nil)
		guest.mustStatus(200, status, body, "unknown lang falls back")
		if d := data(body); d["language"] != "ro" {
			t.Errorf("fallback language = %v, want ro", d["language"])
		}

		// chapters.pages metadata follows the RO set
		status, body = guest.do("GET", fmt.Sprintf("/api/manga/%d/chapters/900", fx.mangaID), nil)
		guest.mustStatus(200, status, body, "chapter meta")
		if data(body)["pages"] != float64(3) {
			t.Errorf("chapter.pages = %v, want 3", data(body)["pages"])
		}

		status, body = admin.do("DELETE", fmt.Sprintf("/api/chapters/%d/pages?lang=en", chID), nil)
		admin.mustStatus(200, status, body, "delete en pages")
		status, body = guest.do("GET", fmt.Sprintf("/api/chapters/%d/pages?lang=en", chID), nil)
		guest.mustStatus(200, status, body, "en gone, ro remains")
		if d := data(body); d["language"] != "ro" || len(d["languages"].([]any)) != 1 {
			t.Errorf("after delete = %v", d)
		}
	})

	//: multipart page-image upload — R2 unconfigured here, so the
	// local UPLOADS_DIR fallback is what's under test (the R2 protocol has its
	// own unit tests in internal/storage).
	t.Run("ChapterPageUpload", func(t *testing.T) {
		admin := &client{t: t, base: srv.URL, ip: "10.1.0.75"}
		admin.signupWithRole("admin")
		guest := &client{t: t, base: srv.URL, ip: "10.1.0.76"}

		status, body := admin.do("POST", fmt.Sprintf("/api/manga/%d/chapters", fx.mangaID),
			map[string]any{"chapterNumber": 902})
		admin.mustStatus(201, status, body, "create chapter")
		chID := int(data(body)["id"].(float64))

		pngBytes := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 64)...)
		upload := func(c *client, files map[string][]byte) (int, map[string]any) {
			t.Helper()
			var buf bytes.Buffer
			mw := multipart.NewWriter(&buf)
			_ = mw.WriteField("lang", "ro")
			// deterministic field order so the filename-sort assertion is real
			names := make([]string, 0, len(files))
			for n := range files {
				names = append(names, n)
			}
			sort.Sort(sort.Reverse(sort.StringSlice(names))) // send out of order
			for _, n := range names {
				fw, _ := mw.CreateFormFile("pages", n)
				fw.Write(files[n]) //nolint:errcheck
			}
			mw.Close()
			req, _ := http.NewRequest("POST", srv.URL+fmt.Sprintf("/api/chapters/%d/pages/upload", chID), &buf)
			req.Header.Set("Content-Type", mw.FormDataContentType())
			req.Header.Set("X-Forwarded-For", c.ip)
			if c.token != "" {
				req.Header.Set("Authorization", "Bearer "+c.token)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			var out map[string]any
			_ = json.NewDecoder(resp.Body).Decode(&out)
			return resp.StatusCode, out
		}

		status, body = upload(guest, map[string][]byte{"01.png": pngBytes})
		guest.mustStatus(401, status, body, "guest cannot upload pages")

		// two pages sent in reverse filename order → stored sorted
		status, body = upload(admin, map[string][]byte{"01.png": pngBytes, "02.png": pngBytes})
		admin.mustStatus(200, status, body, "upload ro pages")
		if body["count"] != float64(2) || body["storage"] != "local" {
			t.Fatalf("upload response = %v", body)
		}
		urls := body["urls"].([]any)
		if u := urls[0].(string); !strings.HasSuffix(u, "/001.png") || !strings.HasPrefix(u, "/uploads/manga/") {
			t.Errorf("first page url = %q", u)
		}

		// the reader sees the edition and the file actually serves
		status, body = guest.do("GET", fmt.Sprintf("/api/chapters/%d/pages", chID), nil)
		guest.mustStatus(200, status, body, "read uploaded pages")
		d := data(body)
		if d["language"] != "ro" || len(d["pages"].([]any)) != 2 {
			t.Fatalf("uploaded pages = %v", d)
		}
		resp, err := http.Get(srv.URL + d["pages"].([]any)[0].(string))
		if err != nil {
			t.Fatal(err)
		}
		served, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || !bytes.Equal(served, pngBytes) {
			t.Errorf("serving uploaded page: status %d, %d bytes", resp.StatusCode, len(served))
		}

		// non-image content is rejected by magic-byte sniffing
		status, body = upload(admin, map[string][]byte{"evil.png": []byte("#!/bin/sh\nrm -rf /")})
		admin.mustStatus(400, status, body, "non-image rejected")
	})

	t.Run("MangaExtractor", func(t *testing.T) {
		admin := &client{t: t, base: srv.URL, ip: "10.1.0.80"}
		admin.signupWithRole("admin")
		guest := &client{t: t, base: srv.URL, ip: "10.1.0.81"}

		status, body := admin.do("POST", fmt.Sprintf("/api/manga/%d/chapters", fx.mangaID),
			map[string]any{"chapterNumber": 901})
		admin.mustStatus(201, status, body, "create chapter")
		chID := int(data(body)["id"].(float64))

		// an EN edition that exists only as a MangaDex source
		status, body = admin.do("POST", fmt.Sprintf("/api/chapters/%d/links", chID), map[string]any{
			"hostingUrl": "https://mangadex.org/chapter/" + testMDChapter,
			"language":   "en", "kind": "extract",
			"provider": "mangadex", "providerRef": testMDChapter,
		})
		admin.mustStatus(201, status, body, "add mangadex source")

		// the reader sees the edition, but only through the image proxy
		status, body = guest.do("GET", fmt.Sprintf("/api/chapters/%d/pages", chID), nil)
		guest.mustStatus(200, status, body, "extracted pages")
		d := data(body)
		if d["language"] != "en" {
			t.Fatalf("language = %v, want en", d["language"])
		}
		pages := d["pages"].([]any)
		if len(pages) != 2 {
			t.Fatalf("pages = %v, want 2 proxied pages", pages)
		}
		want := fmt.Sprintf("/api/chapters/%d/pageimg/0?lang=en", chID)
		if pages[0] != want {
			t.Fatalf("pages[0] = %v, want %s", pages[0], want)
		}

		// the proxy streams the image with the upstream content type
		resp, err := http.Get(srv.URL + want)
		if err != nil {
			t.Fatalf("proxy get: %v", err)
		}
		img, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/png" || string(img) != "PNGDATA" {
			t.Fatalf("proxy = %d %s %q", resp.StatusCode, resp.Header.Get("Content-Type"), img)
		}
		if resp, err = http.Get(srv.URL + fmt.Sprintf("/api/chapters/%d/pageimg/9?lang=en", chID)); err == nil {
			if resp.StatusCode != 404 {
				t.Errorf("out-of-range page = %d, want 404", resp.StatusCode)
			}
			resp.Body.Close()
		}

		// test-source knows manga extractors and reports the page count
		status, body = admin.do("POST", "/api/admin/test-source", map[string]any{
			"kind": "extract", "provider": "mangadex", "providerRef": testMDChapter})
		admin.mustStatus(200, status, body, "test mangadex source")
		if d := data(body); d["ok"] != true || !strings.Contains(d["message"].(string), "2 pagini") {
			t.Errorf("test-source = %v", d)
		}

		// a dead ref is a result, not an error
		status, body = admin.do("POST", "/api/admin/test-source", map[string]any{
			"kind": "extract", "provider": "mangadex",
			"providerRef": "99999999-9999-9999-9999-999999999999"})
		admin.mustStatus(200, status, body, "test dead mangadex source")
		if d := data(body); d["ok"] != false {
			t.Errorf("dead source test = %v", d)
		}

		// our own pages always beat the extractor for the same language
		status, body = admin.do("PUT", fmt.Sprintf("/api/chapters/%d/pages", chID),
			map[string]any{"language": "en", "urls": []string{"https://cdn.example/own/1.jpg"}})
		admin.mustStatus(200, status, body, "set own en pages")
		status, body = guest.do("GET", fmt.Sprintf("/api/chapters/%d/pages?lang=en", chID), nil)
		guest.mustStatus(200, status, body, "own pages win")
		if d := data(body); d["pages"].([]any)[0] != "https://cdn.example/own/1.jpg" {
			t.Errorf("own pages should win over the extractor: %v", d["pages"])
		}
	})

	t.Run("LoginRateLimit", func(t *testing.T) {
		c := &client{t: t, base: srv.URL, ip: "10.99.99.99"} // dedicated bucket
		got429 := false
		for i := 0; i < 12; i++ {
			status, _ := c.do("POST", "/api/auth/login", map[string]any{
				"email": "nobody@test.local", "password": "wrongwrongwrong"})
			if status == http.StatusTooManyRequests {
				got429 = true
				break
			}
		}
		if !got429 {
			t.Error("12 rapid login attempts from one IP never hit 429")
		}
	})
}

// upload posts a multipart avatar.
func (c *client) upload(filename, mimeType string, content []byte) (int, map[string]any) {
	c.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	hdr := make(map[string][]string)
	hdr["Content-Disposition"] = []string{fmt.Sprintf(`form-data; name="avatar"; filename=%q`, filename)}
	hdr["Content-Type"] = []string{mimeType}
	part, _ := mw.CreatePart(hdr)
	part.Write(content)
	mw.Close()

	req, _ := http.NewRequest("POST", c.base+"/api/users/me/avatar", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Forwarded-For", c.ip)
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// promote flips a user's role straight in the database.
func promote(t *testing.T, username, role string) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, `UPDATE users SET role = $2 WHERE username = $1`, username, role); err != nil {
		t.Fatal(err)
	}
}

// signupWithRole registers a fresh user, flips their role in the DB, and
// re-logs-in so the JWT carries it.
func (c *client) signupWithRole(role string) string {
	c.t.Helper()
	username := c.signup()
	promote(c.t, username, role)
	status, body := c.do("POST", "/api/auth/login", map[string]any{
		"email": username + "@test.local", "password": "longpassword123"})
	c.mustStatus(200, status, body, "re-login as "+role)
	c.token = body["token"].(string)
	return username
}

// postForm posts a multipart form: string fields plus files given as
// field → {filename, content}.
func (c *client) postForm(path string, fields map[string]string, files map[string][2]string) (int, map[string]any) {
	c.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	for field, f := range files {
		part, _ := mw.CreateFormFile(field, f[0])
		_, _ = io.WriteString(part, f[1])
	}
	mw.Close()

	req, _ := http.NewRequest("POST", c.base+path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Forwarded-For", c.ip)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// getRaw fetches a non-JSON endpoint (staging video, draft.vtt).
func (c *client) getRaw(path string) (int, string) {
	c.t.Helper()
	req, _ := http.NewRequest("GET", c.base+path, nil)
	req.Header.Set("X-Forwarded-For", c.ip)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}
