package handler

// Curated placements. One public read for the pages, one gated
// write for the admin panel.

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"animekage/backend/internal/httpx"
	"animekage/backend/internal/middleware"
	"animekage/backend/internal/repo"
)

// curatedSlot describes what a placement will accept. The registry lives in
// the server, not the admin page: the UI is one client among several (and the
// next one may be a script), so "3 anime, no manga" has to be enforced where
// it cannot be skipped.
type curatedSlot struct {
	Key   string `json:"key"`
	Label string `json:"label"` // shown in the admin panel
	Hint  string `json:"hint"`
	Max   int    `json:"max"`
	// Media is "anime", "manga", "list", or "" for any title kind. A "list"
	// slot features a member's list rather than a title, so the admin picker
	// and the validation below both branch on it.
	Media string `json:"media"`
}

// The four surfaces the editor controls. Adding one here plus a loader change
// is all a new placement needs.
var curatedSlots = []curatedSlot{
	{
		Key:   "landing_collage",
		Label: "Colaj pagina de start",
		Hint:  "Cele trei coperți din colajul paginii publice. Doar imagini — nu duc nicăieri.",
		Max:   3,
		Media: "", // decorative art, so either kind works
	},
	{
		Key:   "home_spotlight",
		Label: "Spotlight acasă",
		Hint:  "Titlul mare din capul dashboardului. Alege unul cu sinopsis și copertă.",
		Max:   1,
		Media: "anime", // the block links to /anime/:id/episode/1
	},
	{
		Key:   "anime_featured",
		Label: "Recomandarea catalogului (anime)",
		Hint:  "Bannerul din capul paginii /anime.",
		Max:   1,
		Media: "anime",
	},
	{
		Key:   "manga_featured",
		Label: "Recomandarea bibliotecii (manga)",
		Hint:  "Bannerul din capul paginii /manga.",
		Max:   1,
		Media: "manga",
	},
	{
		Key:   "liste_featured",
		Label: "Listă remarcată",
		Hint:  "Lista scoasă în față pe /liste. Alege un membru, apoi una dintre listele lui publice.",
		Max:   1,
		Media: "list",
	},
}

// cleanOverrideURL keeps a placement override to something this server
// actually serves. Anything else — an off-site URL, a javascript: scheme — is
// dropped rather than rejected: the pick itself is still valid, it just falls
// back to the title's real cover. Uploads always land under /uploads/curated/,
// so a legitimate value can never look like anything else.
func cleanOverrideURL(raw *string) *string {
	if raw == nil {
		return nil
	}
	s := strings.TrimSpace(*raw)
	if s == "" || !strings.HasPrefix(s, "/uploads/curated/") || strings.Contains(s, "..") {
		return nil
	}
	return &s
}

func curatedSlotByKey(key string) (curatedSlot, bool) {
	for _, s := range curatedSlots {
		if s.Key == key {
			return s, true
		}
	}
	return curatedSlot{}, false
}

// GET /api/curated
//
// Every slot in one call. The pages that need it are server-rendered and each
// would otherwise make its own request; one payload keeps SSR to a single
// round trip. Public, because the results are public content either way.
func (h *Handler) listCurated(w http.ResponseWriter, r *http.Request) {
	out := make(map[string][]repo.CuratedPick, len(curatedSlots))
	for _, s := range curatedSlots {
		picks, err := h.repo.CuratedSlot(r.Context(), s.Key)
		if err != nil {
			httpx.Internal(w, "list curated picks", err)
			return
		}
		out[s.Key] = picks
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out, "slots": curatedSlots})
}

// POST /api/curated/image — admin/coordinator/moderator.
//
// Stores artwork for a placement and hands back its URL; the editor then
// saves the slot with that URL attached to a pick. Deliberately not tied to a
// title: this never touches `anime.image_url`, so the catalog, the cards and
// every list keep showing the real cover.
func (h *Handler) uploadCuratedImage(w http.ResponseWriter, r *http.Request) {
	h.uploadPoster(w, r, "curated", nil)
}

// PUT /api/curated/{slot} — admin/moderator.
//
// Replaces the slot with the posted list, in the order given. An empty list
// clears it, which is how you go back to the automatic pick.
func (h *Handler) replaceCurated(w http.ResponseWriter, r *http.Request) {
	slotKey := strings.TrimSpace(chi.URLParam(r, "slot"))
	slot, ok := curatedSlotByKey(slotKey)
	if !ok {
		httpx.Error(w, http.StatusNotFound, "Plasament necunoscut")
		return
	}

	var body struct {
		Items []repo.CuratedRef `json:"items"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid input data")
		return
	}

	if len(body.Items) > slot.Max {
		httpx.Error(w, http.StatusBadRequest,
			fmt.Sprintf("Plasamentul „%s” acceptă cel mult %d titluri", slot.Label, slot.Max))
		return
	}
	seen := make(map[string]bool, len(body.Items))
	for _, it := range body.Items {
		media := strings.ToLower(strings.TrimSpace(it.MediaType))
		if media != "anime" && media != "manga" && media != "list" {
			httpx.Error(w, http.StatusBadRequest, "Tip de conținut invalid")
			return
		}
		// A list is only ever valid in a slot declared for lists, and such a
		// slot takes nothing else. Checked explicitly rather than leaning on
		// the generic comparison below, so neither direction can slip through.
		if (media == "list") != (slot.Media == "list") {
			httpx.Error(w, http.StatusBadRequest,
				fmt.Sprintf("Plasamentul „%s” nu acceptă acest tip", slot.Label))
			return
		}
		if slot.Media != "" && media != slot.Media {
			httpx.Error(w, http.StatusBadRequest,
				fmt.Sprintf("Plasamentul „%s” acceptă doar %s", slot.Label, slot.Media))
			return
		}
		if it.ID <= 0 {
			httpx.Error(w, http.StatusBadRequest, "ID invalid")
			return
		}
		// The UNIQUE is on (slot, position), so a duplicate would be stored
		// happily and render the same poster twice.
		key := fmt.Sprintf("%s:%d", media, it.ID)
		if seen[key] {
			httpx.Error(w, http.StatusBadRequest, "Același titlu apare de două ori")
			return
		}
		seen[key] = true
	}

	// normalise before storing so the repo doesn't have to re-trim
	items := make([]repo.CuratedRef, len(body.Items))
	for i, it := range body.Items {
		items[i] = repo.CuratedRef{
			MediaType: strings.ToLower(strings.TrimSpace(it.MediaType)),
			ID:        it.ID,
			ImageURL:  cleanOverrideURL(it.ImageURL),
		}
	}

	user := middleware.UserFrom(r)
	if err := h.repo.ReplaceCuratedSlot(r.Context(), slot.Key, items, user.UserID); err != nil {
		// The FK is the only thing standing between a typo'd id and a slot
		// pointing at nothing, so treat its failure as bad input, not a 500.
		if repo.IsForeignKeyViolation(err) {
			httpx.Error(w, http.StatusBadRequest, "Un titlu selectat nu există în catalog")
			return
		}
		httpx.Internal(w, "replace curated slot", err)
		return
	}

	picks, err := h.repo.CuratedSlot(r.Context(), slot.Key)
	if err != nil {
		httpx.Internal(w, "replace curated slot", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": picks})
}
