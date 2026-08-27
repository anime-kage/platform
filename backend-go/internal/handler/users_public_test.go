package handler_test

// Public profile surfaces. Lists, ratings and reviews were
// already public Letterboxd-style; history joined them so another member's
// page shows the same material your own does.

import (
	"fmt"
	"testing"
)

func TestPublicHistoryIsReadableWithoutAToken(t *testing.T) {
	srv, fx := newTestServer(t)

	owner := &client{t: t, base: srv.URL, ip: "10.13.0.1"}
	name := owner.signup()

	// watching 3 episodes is what writes a history row
	status, body := owner.do("POST", "/api/users/me/watchlist", map[string]any{
		"animeId": fx.animeID, "status": "watching", "episodesWatched": 3,
	})
	owner.mustStatus(201, status, body, "track progress")

	// a guest — no Authorization header at all
	guest := &client{t: t, base: srv.URL, ip: "10.13.0.2"}
	status, body = guest.do("GET", fmt.Sprintf("/api/users/%s/history", name), nil)
	guest.mustStatus(200, status, body, "guest reads public history")

	days, _ := body["data"].([]any)
	if len(days) == 0 {
		t.Fatal("history came back empty — the profile activity chart would be blank")
	}
	first := days[0].(map[string]any)
	if ep, _ := first["episodes"].(float64); ep != 3 {
		t.Fatalf("expected 3 episodes on the day, got %v", first["episodes"])
	}
}

func TestPublicHistoryUnknownUserIs404(t *testing.T) {
	srv, _ := newTestServer(t)
	c := &client{t: t, base: srv.URL, ip: "10.13.0.3"}
	status, body := c.do("GET", "/api/users/nobody_at_all/history", nil)
	c.mustStatus(404, status, body, "history for a user that doesn't exist")
}
