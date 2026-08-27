package handler

// Importing an existing list from AniList or MyAnimeList.

import (
	"fmt"
	"net/http"
	"strings"

	"animekage/backend/internal/httpx"
	"animekage/backend/internal/listimport"
	"animekage/backend/internal/malxml"
	"animekage/backend/internal/middleware"
)

// POST /api/users/me/import/anilist  {"username": "..."}
//
// One GraphQL call per media type. AniList's MediaListCollection is public and
// unauthenticated, so a username is all we need — no OAuth dance, nothing for
// the member to register.
func (h *Handler) importAniList(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid input data")
		return
	}
	username := strings.TrimSpace(body.Username)
	if username == "" {
		httpx.Error(w, http.StatusBadRequest, "Numele de utilizator AniList este obligatoriu")
		return
	}

	user := middleware.UserFrom(r)
	out := map[string]any{}

	for _, kind := range []struct {
		gql   string
		manga bool
		key   string
	}{{"ANIME", false, "anime"}, {"MANGA", true, "manga"}} {
		raw, err := h.anilist.UserList(username, kind.gql)
		if err != nil {
			// AniList says "Private User" / "User not found" through the same
			// channel; both are the member's problem to fix, not a 500.
			msg := err.Error()
			switch {
			case strings.Contains(msg, "Private"):
				httpx.Error(w, http.StatusBadRequest,
					"Lista aceea este privată. Fă-o publică în setările AniList și încearcă din nou.")
			case strings.Contains(msg, "not found"):
				httpx.Error(w, http.StatusBadRequest, "Nu există niciun cont AniList cu acest nume.")
			default:
				httpx.Internal(w, "import anilist list", err)
			}
			return
		}

		entries := make([]listimport.Entry, 0, len(raw))
		for _, e := range raw {
			status, ok := listimport.FromAniList(e.Status, kind.manga)
			if !ok {
				continue // a status we have no equivalent for
			}
			entries = append(entries, listimport.Entry{
				MalID:       e.MalID,
				Title:       e.Title,
				Status:      status,
				Progress:    e.Progress,
				Score:       int(e.Score + 0.5), // POINT_10 comes back as a float
				Notes:       e.Notes,
				StartedAt:   e.StartedAt,
				CompletedAt: e.CompletedAt,
			})
		}

		res, err := h.repo.ImportList(r.Context(), user.UserID, entries, kind.manga)
		if err != nil {
			httpx.Internal(w, "import anilist list", err)
			return
		}
		out[kind.key] = res
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}

// POST /api/users/me/import/mal  (multipart, field "file")
//
// MAL's own export rather than an API: Jikan's /users/* endpoints scrape MAL
// and 504 constantly, and MAL's v2 API needs a registered client id. The
// export also works for private lists, which an API never would.
func (h *Handler) importMAL(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, malxml.MaxSize+4096)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Fișierul este prea mare")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "Lipsește fișierul exportat")
		return
	}
	defer file.Close()

	list, err := malxml.Parse(file)
	if err != nil {
		// the parser's messages are already user-facing Romanian
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	entries := make([]listimport.Entry, 0, len(list.Entries))
	for _, e := range list.Entries {
		status, ok := listimport.FromMAL(e.Status, list.Manga)
		if !ok {
			continue
		}
		entries = append(entries, listimport.Entry{
			MalID:       e.MalID,
			Title:       e.Title,
			Status:      status,
			Progress:    e.Progress,
			Score:       e.Score,
			Notes:       e.Notes,
			StartedAt:   e.StartedAt,
			CompletedAt: e.CompletedAt,
		})
	}

	user := middleware.UserFrom(r)
	res, err := h.repo.ImportList(r.Context(), user.UserID, entries, list.Manga)
	if err != nil {
		httpx.Internal(w, "import mal list", err)
		return
	}

	key := "anime"
	if list.Manga {
		key = "manga"
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{key: res},
		"message": fmt.Sprintf("%d titluri importate, %d actualizate, %d nu sunt în catalog",
			res.Imported, res.Updated, res.Skipped),
	})
}
