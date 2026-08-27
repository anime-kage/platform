package jikan

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func str(s string) *string { return &s }

func TestTransformAnime(t *testing.T) {
	day := "Saturdays"
	j := jikanAnime{
		MalID:  52991,
		Title:  "Sousou no Frieren",
		Status: "Currently Airing",
		Type:   "TV",
		Genres: []named{{"Adventure"}, {"Drama"}},
		Broadcast: &struct {
			Day  *string `json:"day"`
			Time *string `json:"time"`
		}{Day: &day, Time: str("23:00")},
	}
	j.Images.JPG.ImageURL = "small.jpg"
	j.Images.JPG.LargeImageURL = "large.jpg"

	a := transformAnime(j)
	if a.Status != "airing" {
		t.Errorf("status = %q, want airing", a.Status)
	}
	if a.Type != "tv" {
		t.Errorf("type = %q, want tv", a.Type)
	}
	if a.BroadcastDay == nil || *a.BroadcastDay != "saturday" {
		t.Errorf(`broadcast day = %v, want "saturday" (Saturdays → saturday)`, a.BroadcastDay)
	}
	if a.ImageURL == nil || *a.ImageURL != "large.jpg" {
		t.Errorf("image = %v, want the large one preferred", a.ImageURL)
	}
	if len(a.Genres) != 2 || a.Genres[0] != "Adventure" {
		t.Errorf("genres = %v", a.Genres)
	}
}

func TestTransformAnimeFallbacks(t *testing.T) {
	a := transformAnime(jikanAnime{Status: "Something New", Type: "PV"})
	if a.Status != "completed" {
		t.Errorf("unknown status fell back to %q, want completed", a.Status)
	}
	if a.Type != "tv" {
		t.Errorf("unknown type fell back to %q, want tv", a.Type)
	}
	if a.BroadcastDay != nil || a.TrailerURL != nil || a.ImageURL != nil {
		t.Error("nil sub-objects must stay nil, not panic or invent values")
	}

	small := transformAnime(jikanAnime{Images: images{}})
	if small.ImageURL != nil {
		t.Error("empty image URLs must map to nil")
	}
}

func TestTransformAnimeTypeMap(t *testing.T) {
	for in, want := range map[string]string{
		"TV": "tv", "Movie": "movie", "OVA": "ova", "ONA": "ova", "Special": "special",
	} {
		if got := transformAnime(jikanAnime{Type: in}).Type; got != want {
			t.Errorf("type %q → %q, want %q", in, got, want)
		}
	}
}

func TestTransformManga(t *testing.T) {
	j := jikanManga{
		MalID:   2,
		Title:   "Berserk",
		Status:  "Publishing",
		Type:    "Manga",
		Authors: []named{{"Miura, Kentarou"}},
		Published: &struct {
			From *string `json:"from"`
		}{From: str("1989-08-25T00:00:00+00:00")},
	}
	m := transformManga(j)
	if m.Status != "publishing" {
		t.Errorf("status = %q", m.Status)
	}
	if m.Year == nil || *m.Year != 1989 {
		t.Errorf("year = %v, want 1989 parsed from published.from", m.Year)
	}
	if len(m.Authors) != 1 || m.Authors[0] != "Miura, Kentarou" {
		t.Errorf("authors = %v", m.Authors)
	}

	if got := transformManga(jikanManga{Type: "Light Novel"}).Type; got != "novel" {
		t.Errorf("Light Novel → %q, want novel", got)
	}
	if got := transformManga(jikanManga{}).Year; got != nil {
		t.Errorf("missing published.from must give nil year, got %v", got)
	}
}

func TestBuildQueryClamps(t *testing.T) {
	c := NewClient()
	v := c.buildQuery("naruto", SearchOpts{Page: 0, Limit: 100, Type: "tv", Year: 2020})
	if v.Get("page") != "1" {
		t.Errorf("page = %q, want 1 (clamped)", v.Get("page"))
	}
	if v.Get("limit") != "25" {
		t.Errorf("limit = %q, want 25 (Jikan max)", v.Get("limit"))
	}
	if v.Get("start_date") != "2020-01-01" {
		t.Errorf("start_date = %q", v.Get("start_date"))
	}
	if v.Get("q") != "naruto" || v.Get("type") != "tv" {
		t.Errorf("q/type not passed through: %v", v)
	}
}

// newTestClient points the client at a fake Jikan without the real rate delay
// mattering (only a couple of requests per test).
func newTestClient(handler http.Handler) (*Client, *httptest.Server) {
	srv := httptest.NewServer(handler)
	c := NewClient()
	c.baseURL = srv.URL
	c.http = srv.Client()
	c.http.Timeout = 5 * time.Second
	return c, srv
}

func TestClientRetriesOn429(t *testing.T) {
	if testing.Short() {
		t.Skip("retry backoff sleeps; skipped with -short")
	}
	calls := 0
	c, srv := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"data": jikanAnime{MalID: 1, Title: "ok"}})
	}))
	defer srv.Close()

	a, err := c.AnimeByID(1)
	if err != nil {
		t.Fatalf("AnimeByID after 429: %v", err)
	}
	if a.Title != "ok" || calls != 2 {
		t.Errorf("title=%q calls=%d, want retry then success", a.Title, calls)
	}
}

func TestClientErrors(t *testing.T) {
	c, srv := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	if _, err := c.AnimeByID(999999999); err == nil {
		t.Error("404 must surface as an error")
	}
}

func TestTopAnimeUsesFilterParam(t *testing.T) {
	// regression: the TS backend sent ?type= which Jikan silently ignored
	var gotFilter string
	c, srv := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotFilter = r.URL.Query().Get("filter")
		json.NewEncoder(w).Encode(map[string]any{"data": []jikanAnime{}})
	}))
	defer srv.Close()

	if _, _, err := c.TopAnime("airing", 1, 10); err != nil {
		t.Fatal(err)
	}
	if gotFilter != "airing" {
		t.Errorf("filter param = %q, want airing", gotFilter)
	}
}

// An empty `data` object with a 200 status must be an error, not a zero-valued
// record. Regression test for the bug that blanked a live anime row: the sync
// writes whatever this returns straight into the catalog, and transformAnime
// maps {} to a plausible-looking row (MalID 0, empty Title, Status "completed"
// from mapOr's default) rather than to something obviously invalid. The
// resulting mal_id=0 row then collided with every later occurrence, which is
// the anime_mal_id_unique violation the nightly autoupdate logged for weeks.
func TestAnimeByIDRejectsEmptyPayload(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"empty object", `{"data":{}}`},
		{"null data", `{"data":null}`},
		{"no mal id", `{"data":{"title":"Something"}}`},
		{"no title", `{"data":{"mal_id":52991}}`},
		{"blank title", `{"data":{"mal_id":52991,"title":"   "}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, srv := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			if _, err := c.AnimeByID(52991); err == nil {
				t.Fatal("want an error for an empty payload, got nil")
			} else if !errors.Is(err, ErrEmptyPayload) {
				t.Fatalf("want ErrEmptyPayload, got %v", err)
			}
		})
	}
}

// The happy path must still come through untouched.
func TestAnimeByIDAcceptsRealPayload(t *testing.T) {
	c, srv := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"mal_id":52991,"title":"Sousou no Frieren","status":"Finished Airing"}}`))
	}))
	defer srv.Close()

	a, err := c.AnimeByID(52991)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.MalID != 52991 || a.Title != "Sousou no Frieren" {
		t.Fatalf("got %+v", a)
	}
}

func TestMangaByIDRejectsEmptyPayload(t *testing.T) {
	c, srv := newTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{}}`))
	}))
	defer srv.Close()

	if _, err := c.MangaByID(126287); !errors.Is(err, ErrEmptyPayload) {
		t.Fatalf("want ErrEmptyPayload, got %v", err)
	}
}
