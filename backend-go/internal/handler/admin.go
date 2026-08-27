package handler

// Admin panel endpoints. test-source lets a coordinator prove
// a source plays BEFORE saving it; the health report surfaces what the health
// checker (3.8) and the subtitle pipeline already know.

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"

	"animekage/backend/internal/httpx"
	"animekage/backend/internal/resolver"
)

type testSourceBody struct {
	Kind        string  `json:"kind"` // 'embed' (default) | 'extract'
	HostingURL  string  `json:"hostingUrl"`
	Provider    *string `json:"provider"`
	ProviderRef *string `json:"providerRef"`
}

// POST /api/admin/test-source  (admin/translator)
//
// Always 200 with {ok, message?, ...} — a dead source is a result, not an
// HTTP error. Embeds can only be validated (an iframe's liveness isn't ours
// to probe); extract sources are resolved and their manifest fetched, the
// same check the health checker runs.
func (h *Handler) testSource(w http.ResponseWriter, r *http.Request) {
	var body testSourceBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid input data")
		return
	}
	if body.Kind == "" {
		body.Kind = "embed"
	}

	fail := func(msg string) {
		httpx.JSON(w, http.StatusOK, map[string]any{"data": map[string]any{"ok": false, "message": msg}})
	}

	switch body.Kind {
	case "embed":
		if body.HostingURL == "" {
			httpx.Error(w, http.StatusBadRequest, "hostingUrl is required for embeds")
			return
		}
		if err := validateHostingURL(body.HostingURL, h.cfg.ContentHosts); err != nil {
			fail(err.Error())
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"data": map[string]any{
			"ok": true, "message": "URL valid (embeds can't be probed further)",
		}})
	case "extract":
		if body.Provider == nil || *body.Provider == "" || body.ProviderRef == nil || *body.ProviderRef == "" {
			httpx.Error(w, http.StatusBadRequest, "extract sources need provider and providerRef")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		// manga page extractors report a page count instead of a manifest
		if h.mangaext.Has(*body.Provider) {
			res, err := h.mangaext.Extract(ctx, *body.Provider, *body.ProviderRef)
			if err != nil {
				fail("Extragerea a eșuat: " + err.Error())
				return
			}
			httpx.JSON(w, http.StatusOK, map[string]any{"data": map[string]any{
				"ok": true, "message": fmt.Sprintf("ok · %d pagini", len(res.Pages)),
			}})
			return
		}
		res, err := h.resolver.Resolve(ctx, *body.Provider, *body.ProviderRef)
		if err != nil {
			fail("Rezolvarea a eșuat: " + err.Error())
			return
		}
		if err := resolver.Probe(ctx, res); err != nil {
			fail("Sursa nu răspunde: " + err.Error())
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"data": map[string]any{
			"ok": true, "manifestUrl": res.ManifestURL, "streamKind": res.Kind,
		}})
	default:
		httpx.Error(w, http.StatusBadRequest, "kind must be 'embed' or 'extract'")
	}
}

// GET /api/admin/episodes/{id}/links  (admin/translator) — every link,
// including disabled ones (the public episode endpoints filter those out).
func (h *Handler) adminEpisodeLinks(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid episode ID")
		return
	}
	links, err := h.repo.AllEpisodeLinks(r.Context(), id)
	if err != nil {
		httpx.Internal(w, "fetch episode links", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": links})
}

// GET /api/admin/overview  (admin/moderator) — the panel's stat strip.
func (h *Handler) adminOverview(w http.ResponseWriter, r *http.Request) {
	o, err := h.repo.AdminOverview(r.Context())
	if err != nil {
		httpx.Internal(w, "build admin overview", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": o})
}

// GET /api/admin/team  (admin/moderator) — everyone with a team role.
func (h *Handler) adminTeam(w http.ResponseWriter, r *http.Request) {
	rows, err := h.repo.TeamMembers(r.Context())
	if err != nil {
		httpx.Internal(w, "list team", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": rows})
}

// GET /api/admin/health-gaps?kind=source|rosub&limit=&offset=  (admin/translator)
// One page of a health-report gap list. The report embeds only the first page
// because these run to thousands of rows; this is what "vezi toate" walks.
func (h *Handler) adminHealthGaps(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	limit := httpx.QueryInt(r, "limit", 50, 1, 200)
	offset := httpx.QueryInt(r, "offset", 0, 0, 1_000_000)

	list, err := h.repo.EpisodeGaps(r.Context(), kind, limit, offset)
	if err != nil {
		notFoundOr(w, err, "kind must be 'source' or 'rosub'", "list health gaps")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": list})
}

// GET /api/admin/storage  (admin/translator) — how much room is left for
// uploads. Staging is the only thing on the box that grows without a ceiling,
// so it gets its own number next to the filesystem's.
func (h *Handler) adminStorage(w http.ResponseWriter, r *http.Request) {
	total, free, err := diskUsage(h.cfg.StagingDir)
	if err != nil {
		// a missing staging dir is normal on a fresh install
		slog.Warn("stat staging filesystem", "dir", h.cfg.StagingDir, "err", err)
	}

	var staging int64
	var releases int
	if err := filepath.WalkDir(h.cfg.StagingDir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entries must not fail the whole report
		}
		if d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			staging += info.Size()
		}
		return nil
	}); err != nil {
		slog.Warn("walk staging dir", "err", err)
	}
	if entries, err := os.ReadDir(h.cfg.StagingDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				releases++
			}
		}
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"stagingBytes":   staging,
		"stagingDirs":    releases,
		"diskTotalBytes": total,
		"diskFreeBytes":  free,
	}})
}

// GET /api/admin/health-report  (admin/translator)
func (h *Handler) adminHealthReport(w http.ResponseWriter, r *http.Request) {
	limit := httpx.QueryInt(r, "limit", 50, 1, 200)
	rep, err := h.repo.HealthReport(r.Context(), limit)
	if err != nil {
		httpx.Internal(w, "build health report", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": rep})
}
