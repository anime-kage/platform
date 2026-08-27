package handler_test

// Curated placements. These four slots are the front page, so the
// rules that matter are the ones that stop a bad write reaching it: who may
// write, how many titles fit, which media type belongs, and what happens when
// a chosen title later leaves the catalog.

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestCuratedIsPublicToReadAndGatedToWrite(t *testing.T) {
	srv, fx := newTestServer(t)

	// public read, no token at all
	guest := &client{t: t, base: srv.URL, ip: "10.12.0.1"}
	status, body := guest.do("GET", "/api/curated", nil)
	guest.mustStatus(200, status, body, "guest reads curated")
	if _, ok := body["slots"]; !ok {
		t.Fatal("response carries no slot registry — the admin UI depends on it")
	}

	// a signed-in member is still not an editor
	member := &client{t: t, base: srv.URL, ip: "10.12.0.2"}
	member.signup()
	status, body = member.do("PUT", "/api/curated/anime_featured", map[string]any{
		"items": []map[string]any{{"mediaType": "anime", "id": fx.animeID}},
	})
	member.mustStatus(403, status, body, "member writes curated")

	// a guest even less so
	status, body = guest.do("PUT", "/api/curated/anime_featured", map[string]any{
		"items": []map[string]any{{"mediaType": "anime", "id": fx.animeID}},
	})
	if status != 401 && status != 403 {
		t.Fatalf("guest write returned %d, want 401/403 (body %v)", status, body)
	}
}

func TestCuratedReplaceAndClear(t *testing.T) {
	srv, fx := newTestServer(t)
	c := &client{t: t, base: srv.URL, ip: "10.12.0.3"}
	c.signupWithRole("coordinator")

	// set
	status, body := c.do("PUT", "/api/curated/anime_featured", map[string]any{
		"items": []map[string]any{{"mediaType": "anime", "id": fx.animeID}},
	})
	c.mustStatus(200, status, body, "coordinator sets the anime banner")

	// it comes back resolved to a full title, not a bare id — the pages
	// render straight from this
	status, body = c.do("GET", "/api/curated", nil)
	c.mustStatus(200, status, body, "read back")
	picks := data(body)["anime_featured"].([]any)
	if len(picks) != 1 {
		t.Fatalf("expected 1 pick, got %d", len(picks))
	}
	first := picks[0].(map[string]any)
	anime, ok := first["anime"].(map[string]any)
	if !ok || anime["title"] == nil {
		t.Fatalf("pick did not resolve to a title: %v", first)
	}

	// replace with a different title — the slot holds one, not two
	status, body = c.do("PUT", "/api/curated/anime_featured", map[string]any{
		"items": []map[string]any{{"mediaType": "anime", "id": fx.anime2ID}},
	})
	c.mustStatus(200, status, body, "replace the banner")
	status, body = c.do("GET", "/api/curated", nil)
	picks = data(body)["anime_featured"].([]any)
	if len(picks) != 1 {
		t.Fatalf("replace left %d picks, want 1", len(picks))
	}

	// empty list clears it, which is how you go back to automatic
	status, body = c.do("PUT", "/api/curated/anime_featured", map[string]any{
		"items": []map[string]any{},
	})
	c.mustStatus(200, status, body, "clear the banner")
	status, body = c.do("GET", "/api/curated", nil)
	if picks = data(body)["anime_featured"].([]any); len(picks) != 0 {
		t.Fatalf("clear left %d picks, want 0", len(picks))
	}
}

// The slot registry lives on the server precisely so these cannot be bypassed
// by anything that isn't the admin page.
func TestCuratedSlotRulesAreEnforced(t *testing.T) {
	srv, fx := newTestServer(t)
	c := &client{t: t, base: srv.URL, ip: "10.12.0.4"}
	c.signupWithRole("coordinator")

	t.Run("over capacity", func(t *testing.T) {
		status, body := c.do("PUT", "/api/curated/home_spotlight", map[string]any{
			"items": []map[string]any{
				{"mediaType": "anime", "id": fx.animeID},
				{"mediaType": "anime", "id": fx.anime2ID},
			},
		})
		c.mustStatus(400, status, body, "two titles in a one-title slot")
	})

	t.Run("wrong media type", func(t *testing.T) {
		// the home spotlight links to /anime/:id/episode/1 — a manga there
		// would render a dead button
		status, body := c.do("PUT", "/api/curated/home_spotlight", map[string]any{
			"items": []map[string]any{{"mediaType": "manga", "id": fx.mangaID}},
		})
		c.mustStatus(400, status, body, "manga in an anime-only slot")
	})

	t.Run("duplicate title", func(t *testing.T) {
		status, body := c.do("PUT", "/api/curated/landing_collage", map[string]any{
			"items": []map[string]any{
				{"mediaType": "anime", "id": fx.animeID},
				{"mediaType": "anime", "id": fx.animeID},
			},
		})
		c.mustStatus(400, status, body, "same poster twice")
	})

	t.Run("nonexistent title", func(t *testing.T) {
		// the FK is what keeps a slot from pointing at nothing; this must be
		// a 400, not the 500 an unhandled constraint error would give
		status, body := c.do("PUT", "/api/curated/anime_featured", map[string]any{
			"items": []map[string]any{{"mediaType": "anime", "id": 987654}},
		})
		c.mustStatus(400, status, body, "id that isn't in the catalog")
	})

	t.Run("unknown slot", func(t *testing.T) {
		status, body := c.do("PUT", "/api/curated/not_a_slot", map[string]any{
			"items": []map[string]any{{"mediaType": "anime", "id": fx.animeID}},
		})
		c.mustStatus(404, status, body, "slot that doesn't exist")
	})

	// the landing collage takes either kind — it is decorative art
	t.Run("mixed media allowed on the collage", func(t *testing.T) {
		status, body := c.do("PUT", "/api/curated/landing_collage", map[string]any{
			"items": []map[string]any{
				{"mediaType": "anime", "id": fx.animeID},
				{"mediaType": "manga", "id": fx.mangaID},
			},
		})
		c.mustStatus(200, status, body, "anime + manga on the collage")
	})
}

// Order is the editor's, not the database's — the collage has three fixed
// positions and which poster sits where is a design decision.
func TestCuratedPreservesOrder(t *testing.T) {
	srv, fx := newTestServer(t)
	c := &client{t: t, base: srv.URL, ip: "10.12.0.5"}
	c.signupWithRole("coordinator")

	status, body := c.do("PUT", "/api/curated/landing_collage", map[string]any{
		"items": []map[string]any{
			{"mediaType": "anime", "id": fx.anime2ID},
			{"mediaType": "anime", "id": fx.animeID},
		},
	})
	c.mustStatus(200, status, body, "set collage order")

	status, body = c.do("GET", "/api/curated", nil)
	picks := data(body)["landing_collage"].([]any)
	if len(picks) != 2 {
		t.Fatalf("expected 2 picks, got %d", len(picks))
	}
	firstID := int(picks[0].(map[string]any)["anime"].(map[string]any)["id"].(float64))
	if firstID != fx.anime2ID {
		t.Fatalf("order not preserved: first pick is %d, want %d", firstID, fx.anime2ID)
	}
}

// A curated title that is later deleted must vanish from the slot rather than
// leave it pointing at a missing row — otherwise the home page 500s the next
// time someone deletes an anime.
func TestCuratedPickDisappearsWithItsTitle(t *testing.T) {
	srv, fx := newTestServer(t)
	c := &client{t: t, base: srv.URL, ip: "10.12.0.6"}
	c.signupWithRole("coordinator")

	status, body := c.do("PUT", "/api/curated/anime_featured", map[string]any{
		"items": []map[string]any{{"mediaType": "anime", "id": fx.anime2ID}},
	})
	c.mustStatus(200, status, body, "curate a title")

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, `DELETE FROM anime WHERE id = $1`, fx.anime2ID); err != nil {
		t.Fatalf("delete anime: %v", err)
	}

	status, body = c.do("GET", "/api/curated", nil)
	c.mustStatus(200, status, body, "read curated after the title was deleted")
	if picks := data(body)["anime_featured"].([]any); len(picks) != 0 {
		t.Fatalf("deleted title still curated: %v", picks)
	}
}

// The per-placement image is an override, not an edit: the banner shows the
// uploaded artwork while the anime keeps its own cover everywhere else. It
// also has to survive a reorder, because saving a slot replaces every row.
func TestCuratedImageOverrideIsScopedAndSurvivesReorder(t *testing.T) {
	srv, fx := newTestServer(t)
	c := &client{t: t, base: srv.URL, ip: "10.12.0.7"}
	c.signupWithRole("coordinator")

	const art = "/uploads/curated/banner.png"
	status, body := c.do("PUT", "/api/curated/landing_collage", map[string]any{
		"items": []map[string]any{
			{"mediaType": "anime", "id": fx.animeID, "imageUrl": art},
			{"mediaType": "anime", "id": fx.anime2ID},
		},
	})
	c.mustStatus(200, status, body, "set a placement image")

	// the anime's own poster must be untouched
	status, body = c.do("GET", fmt.Sprintf("/api/anime/%d", fx.animeID), nil)
	c.mustStatus(200, status, body, "read the anime back")
	if got, _ := data(body)["imageUrl"].(string); got == art {
		t.Fatal("placement artwork leaked onto the anime's own poster")
	}

	// reorder: the artwork must follow its pick, not stay at position 0
	status, body = c.do("PUT", "/api/curated/landing_collage", map[string]any{
		"items": []map[string]any{
			{"mediaType": "anime", "id": fx.anime2ID},
			{"mediaType": "anime", "id": fx.animeID, "imageUrl": art},
		},
	})
	c.mustStatus(200, status, body, "reorder with the image carried")

	status, body = c.do("GET", "/api/curated", nil)
	picks := data(body)["landing_collage"].([]any)
	if len(picks) != 2 {
		t.Fatalf("expected 2 picks, got %d", len(picks))
	}
	if url, _ := picks[0].(map[string]any)["imageUrl"].(string); url != "" {
		t.Fatalf("first pick should have no override, got %q", url)
	}
	if url, _ := picks[1].(map[string]any)["imageUrl"].(string); url != art {
		t.Fatalf("override did not follow its pick: got %q, want %q", url, art)
	}
}

// The stored value ends up in an <img src> on the public front page, so
// anything that isn't one of our own uploads is dropped rather than trusted.
func TestCuratedImageOverrideRejectsForeignURLs(t *testing.T) {
	srv, fx := newTestServer(t)
	c := &client{t: t, base: srv.URL, ip: "10.12.0.8"}
	c.signupWithRole("coordinator")

	for _, bad := range []string{
		"https://evil.example/art.png",
		"javascript:alert(1)",
		"/uploads/curated/../../etc/passwd",
		"/uploads/avatars/someone.png",
	} {
		status, body := c.do("PUT", "/api/curated/anime_featured", map[string]any{
			"items": []map[string]any{{"mediaType": "anime", "id": fx.animeID, "imageUrl": bad}},
		})
		// the pick is still valid — only the bad URL is discarded
		c.mustStatus(200, status, body, "save with "+bad)

		status, body = c.do("GET", "/api/curated", nil)
		picks := data(body)["anime_featured"].([]any)
		if url, _ := picks[0].(map[string]any)["imageUrl"].(string); url != "" {
			t.Fatalf("stored a foreign image URL %q (from %q)", url, bad)
		}
	}
}
