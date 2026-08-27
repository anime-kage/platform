package handler

// User profile, follows, history, and the watch/read lists. Public profiles
// and lists are Letterboxd-style: anyone can view, only the owner writes.

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"animekage/backend/internal/httpx"
	"animekage/backend/internal/middleware"
	"animekage/backend/internal/model"
	"animekage/backend/internal/repo"
)

var handleRe = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// GET /api/users/me
func (h *Handler) myProfile(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserFrom(r).UserID
	user, err := h.repo.UserByID(r.Context(), userID)
	if err != nil {
		notFoundOr(w, err, "User not found", "fetch profile")
		return
	}
	stats, err := h.repo.UserStats(r.Context(), userID)
	if err != nil {
		httpx.Internal(w, "fetch profile", err)
		return
	}
	// nil when nothing is chosen (or the chosen title lost its art) — the
	// header then renders in its plain form
	banner, err := h.repo.UserBanner(r.Context(), userID)
	if err != nil {
		httpx.Internal(w, "fetch profile", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"user": user, "stats": stats, "banner": banner})
}

// PUT /api/users/me
func (h *Handler) updateMyProfile(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserFrom(r).UserID
	var body struct {
		Username       *string             `json:"username"`
		Bio            *string             `json:"bio"`
		FavoriteGenres []string            `json:"favoriteGenres"`
		Favorites      []model.FavoriteRef `json:"favorites"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid input data")
		return
	}

	upd := repo.ProfileUpdate{FavoriteGenres: body.FavoriteGenres}
	if body.Username != nil {
		u := *body.Username
		if len(u) < 3 || len(u) > 50 {
			httpx.Error(w, http.StatusBadRequest, "Username must be between 3 and 50 characters")
			return
		}
		if !handleRe.MatchString(u) {
			httpx.Error(w, http.StatusBadRequest, "Username can only contain letters, numbers, and underscores")
			return
		}
		upd.Username = &u
	}
	if body.Bio != nil {
		b := *body.Bio
		if r := []rune(b); len(r) > 500 {
			b = string(r[:500])
		}
		upd.Bio = &b
	}
	if len(upd.FavoriteGenres) > 10 {
		upd.FavoriteGenres = upd.FavoriteGenres[:10]
	}
	if body.Favorites != nil {
		valid := []model.FavoriteRef{}
		for _, f := range body.Favorites {
			if (f.Type == "anime" || f.Type == "manga") && f.ID > 0 && len(valid) < 5 {
				valid = append(valid, f)
			}
		}
		upd.Favorites = valid
	}

	user, err := h.repo.UpdateProfile(r.Context(), userID, upd)
	if err != nil {
		if repo.IsUniqueViolation(err) {
			httpx.Error(w, http.StatusConflict, "Username is already taken")
			return
		}
		httpx.Internal(w, "update profile", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"user": user, "message": "Profile updated successfully"})
}

// avatarExt maps a sniffed content type to a file extension. The old backend
// trusted the client's MIME type and filename; here the bytes decide.
var avatarExt = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
	"image/gif":  "gif",
}

// POST /api/users/me/avatar
func (h *Handler) uploadAvatar(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserFrom(r).UserID

	const maxSize = 2 << 20 // 2 MB
	r.Body = http.MaxBytesReader(w, r.Body, maxSize+4096)
	if err := r.ParseMultipartForm(maxSize); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Avatar must be under 2MB")
		return
	}
	file, header, err := r.FormFile("avatar")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "Avatar file is required")
		return
	}
	defer file.Close()
	if header.Size > maxSize {
		httpx.Error(w, http.StatusBadRequest, "Avatar must be under 2MB")
		return
	}

	data, err := io.ReadAll(file)
	if err != nil {
		httpx.Internal(w, "upload avatar", err)
		return
	}
	ext, ok := avatarExt[http.DetectContentType(data)]
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Only JPEG, PNG, WebP, and GIF images are allowed")
		return
	}

	dir := filepath.Join(h.cfg.UploadsDir, "avatars")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		httpx.Internal(w, "upload avatar", err)
		return
	}
	filename := fmt.Sprintf("%d-%s.%s", userID, uuid.NewString(), ext)
	if err := os.WriteFile(filepath.Join(dir, filename), data, 0o644); err != nil {
		httpx.Internal(w, "upload avatar", err)
		return
	}

	avatarURL := "/uploads/avatars/" + filename
	user, err := h.repo.UpdateProfile(r.Context(), userID, repo.ProfileUpdate{AvatarURL: &avatarURL})
	if err != nil {
		httpx.Internal(w, "upload avatar", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"user":      user,
		"avatarUrl": avatarURL,
		"message":   "Avatar updated successfully",
	})
}

// GET /api/users/me/history?days=14
func (h *Handler) myHistory(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserFrom(r).UserID
	days := httpx.QueryInt(r, "days", 14, 1, 90)
	data, err := h.repo.History(r.Context(), userID, days)
	if err != nil {
		httpx.Internal(w, "fetch history", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": data})
}

// GET /api/users/{username}/history — public watch/read activity.
//
// Public for the same reason the watchlist and reviews are (Letterboxd-style):
// what a member has been watching is the interesting part of their profile,
// and hiding it would make everyone else's page a shell of their own.
func (h *Handler) publicHistory(w http.ResponseWriter, r *http.Request) {
	user, err := h.repo.UserByUsername(r.Context(), chi.URLParam(r, "username"))
	if err != nil {
		notFoundOr(w, err, "User not found", "fetch history")
		return
	}
	days := httpx.QueryInt(r, "days", 14, 1, 90)
	data, err := h.repo.History(r.Context(), user.ID, days)
	if err != nil {
		httpx.Internal(w, "fetch history", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": data})
}

// GET /api/users/{username} — public profile, email excluded
func (h *Handler) publicProfile(w http.ResponseWriter, r *http.Request) {
	user, err := h.repo.UserByUsername(r.Context(), chi.URLParam(r, "username"))
	if err != nil {
		notFoundOr(w, err, "User not found", "fetch profile")
		return
	}
	user.Email = "" // omitted from JSON via omitempty

	stats, err := h.repo.UserStats(r.Context(), user.ID)
	if err != nil {
		httpx.Internal(w, "fetch profile", err)
		return
	}
	followers, following, err := h.repo.FollowCounts(r.Context(), user.ID)
	if err != nil {
		httpx.Internal(w, "fetch profile", err)
		return
	}
	isFollowing := false
	if v := viewerID(r); v != nil && *v != user.ID {
		isFollowing, err = h.repo.IsFollowing(r.Context(), *v, user.ID)
		if err != nil {
			httpx.Internal(w, "fetch profile", err)
			return
		}
	}
	// the backdrop is part of how a profile presents itself, so visitors see
	// it too — not just the owner
	banner, err := h.repo.UserBanner(r.Context(), user.ID)
	if err != nil {
		httpx.Internal(w, "fetch profile", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"user":   user,
		"stats":  stats,
		"banner": banner,
		"network": model.FollowNetwork{
			Followers: followers, Following: following, IsFollowing: isFollowing,
		},
	})
}

// ── Follows ───────────────────────────────────────────────────────────────────

// POST /api/users/{username}/follow
func (h *Handler) follow(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserFrom(r).UserID
	target, err := h.repo.UserByUsername(r.Context(), chi.URLParam(r, "username"))
	if err != nil {
		notFoundOr(w, err, "User not found", "follow user")
		return
	}
	if target.ID == userID {
		httpx.Error(w, http.StatusBadRequest, "Cannot follow yourself")
		return
	}
	if err := h.repo.Follow(r.Context(), userID, target.ID); err != nil {
		httpx.Internal(w, "follow user", err)
		return
	}
	actor := middleware.UserFrom(r)
	link := "/user/" + actor.Username
	h.notify(r.Context(), target.ID, "follow",
		actor.Username+" a început să te urmărească.", &userID, &link)
	h.respondNetwork(w, r, target.ID, true)
}

// DELETE /api/users/{username}/follow
func (h *Handler) unfollow(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserFrom(r).UserID
	target, err := h.repo.UserByUsername(r.Context(), chi.URLParam(r, "username"))
	if err != nil {
		notFoundOr(w, err, "User not found", "unfollow user")
		return
	}
	if err := h.repo.Unfollow(r.Context(), userID, target.ID); err != nil {
		httpx.Internal(w, "unfollow user", err)
		return
	}
	h.respondNetwork(w, r, target.ID, false)
}

func (h *Handler) respondNetwork(w http.ResponseWriter, r *http.Request, targetID int, isFollowing bool) {
	followers, following, err := h.repo.FollowCounts(r.Context(), targetID)
	if err != nil {
		httpx.Internal(w, "fetch follow counts", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"isFollowing": isFollowing,
		"followers":   followers,
		"following":   following,
	}})
}

// GET /api/users/{username}/followers | /following
func (h *Handler) followList(w http.ResponseWriter, r *http.Request, kind string) {
	target, err := h.repo.UserByUsername(r.Context(), chi.URLParam(r, "username"))
	if err != nil {
		notFoundOr(w, err, "User not found", "fetch "+kind)
		return
	}
	viewer := 0
	if v := viewerID(r); v != nil {
		viewer = *v
	}
	data, err := h.repo.FollowList(r.Context(), target.ID, kind, viewer)
	if err != nil {
		httpx.Internal(w, "fetch "+kind, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": data})
}

// GET /api/users/{username}/reviews
func (h *Handler) userReviews(w http.ResponseWriter, r *http.Request) {
	target, err := h.repo.UserByUsername(r.Context(), chi.URLParam(r, "username"))
	if err != nil {
		notFoundOr(w, err, "User not found", "fetch reviews")
		return
	}
	data, err := h.repo.UserReviews(r.Context(), target.ID)
	if err != nil {
		httpx.Internal(w, "fetch reviews", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": data})
}

// ── Watchlist ─────────────────────────────────────────────────────────────────

var watchStatuses = []string{"watching", "completed", "on-hold", "dropped", "plan-to-watch"}
var readStatuses = []string{"reading", "completed", "on-hold", "dropped", "plan-to-read"}

type listBody struct {
	AnimeID         *int    `json:"animeId"`
	MangaID         *int    `json:"mangaId"`
	Status          string  `json:"status"`
	Score           *int    `json:"score"`
	EpisodesWatched *int    `json:"episodesWatched"`
	ChaptersRead    *int    `json:"chaptersRead"`
	VolumesRead     *int    `json:"volumesRead"`
	Notes           *string `json:"notes"`
}

// validScore accepts 1..10, and 0 as "remove my rating".
//
// 0 is not a rating on a 1..10 scale, so it is unambiguous as a sentinel. It
// exists because there was previously no way to un-rate a title at all: the
// repo writes `score = coalesce($4, score)`, so omitting the field means
// "leave it alone" and there was nothing that meant "clear it". The only way
// out was to delete the whole list entry, losing progress and status with it.
func (b listBody) validScore(w http.ResponseWriter) bool {
	if b.Score != nil && (*b.Score < 0 || *b.Score > 10) {
		httpx.Error(w, http.StatusBadRequest, "score must be between 1 and 10, or 0 to remove it")
		return false
	}
	return true
}

// GET /api/users/me/watchlist?status=watching
func (h *Handler) myWatchlist(w http.ResponseWriter, r *http.Request) {
	h.respondWatchlist(w, r, middleware.UserFrom(r).UserID)
}

// GET /api/users/me/continue — the home row. Separate from the watchlist
// because it answers a different question: not "what am I following" but
// "what do I press play on", which is a fact about playback.
func (h *Handler) myContinueWatching(w http.ResponseWriter, r *http.Request) {
	limit := httpx.QueryInt(r, "limit", 12, 1, 40)
	rows, err := h.repo.ContinueWatching(r.Context(), middleware.UserFrom(r).UserID, limit)
	if err != nil {
		httpx.Internal(w, "build continue watching", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": rows})
}

// GET /api/users/{username}/watchlist — public
func (h *Handler) publicWatchlist(w http.ResponseWriter, r *http.Request) {
	target, err := h.repo.UserByUsername(r.Context(), chi.URLParam(r, "username"))
	if err != nil {
		notFoundOr(w, err, "User not found", "fetch watchlist")
		return
	}
	h.respondWatchlist(w, r, target.ID)
}

func (h *Handler) respondWatchlist(w http.ResponseWriter, r *http.Request, userID int) {
	entries, err := h.repo.Watchlist(r.Context(), userID, r.URL.Query().Get("status"))
	if err != nil {
		httpx.Internal(w, "fetch watchlist", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": entries})
}

// GET /api/users/me/watchlist/{animeId}
func (h *Handler) myWatchlistEntry(w http.ResponseWriter, r *http.Request) {
	animeID, ok := httpx.IntParam(chi.URLParam(r, "animeId"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid anime ID")
		return
	}
	entry, err := h.repo.WatchlistEntry(r.Context(), middleware.UserFrom(r).UserID, animeID)
	if err != nil {
		notFoundOr(w, err, "Not in watchlist", "fetch watchlist entry")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": entry})
}

// POST /api/users/me/watchlist
func (h *Handler) upsertWatchlist(w http.ResponseWriter, r *http.Request) {
	var body listBody
	if err := httpx.Decode(r, &body); err != nil || body.AnimeID == nil {
		httpx.Error(w, http.StatusBadRequest, "animeId is required")
		return
	}
	if !slices.Contains(watchStatuses, body.Status) {
		httpx.Error(w, http.StatusBadRequest, "status must be one of: "+strings.Join(watchStatuses, ", "))
		return
	}
	if !body.validScore(w) {
		return
	}
	entry, err := h.repo.UpsertWatchlist(r.Context(), middleware.UserFrom(r).UserID, *body.AnimeID, repo.ListUpsert{
		Status: body.Status, Score: body.Score, Progress: body.EpisodesWatched, Notes: body.Notes,
	})
	if err != nil {
		notFoundOr(w, err, err.Error(), "update watchlist")
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"data": entry, "message": "Watchlist updated"})
}

// PUT /api/users/me/watchlist/{animeId}
func (h *Handler) updateWatchlist(w http.ResponseWriter, r *http.Request) {
	animeID, ok := httpx.IntParam(chi.URLParam(r, "animeId"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid anime ID")
		return
	}
	var body listBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid input data")
		return
	}
	if body.Status == "" {
		body.Status = "watching"
	} else if !slices.Contains(watchStatuses, body.Status) {
		httpx.Error(w, http.StatusBadRequest, "status must be one of: "+strings.Join(watchStatuses, ", "))
		return
	}
	if !body.validScore(w) {
		return
	}
	entry, err := h.repo.UpsertWatchlist(r.Context(), middleware.UserFrom(r).UserID, animeID, repo.ListUpsert{
		Status: body.Status, Score: body.Score, Progress: body.EpisodesWatched, Notes: body.Notes,
	})
	if err != nil {
		notFoundOr(w, err, err.Error(), "update watchlist entry")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": entry, "message": "Watchlist updated"})
}

// DELETE /api/users/me/watchlist/{animeId}
func (h *Handler) removeWatchlist(w http.ResponseWriter, r *http.Request) {
	animeID, ok := httpx.IntParam(chi.URLParam(r, "animeId"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid anime ID")
		return
	}
	removed, err := h.repo.RemoveWatchlist(r.Context(), middleware.UserFrom(r).UserID, animeID)
	if err != nil {
		httpx.Internal(w, "remove from watchlist", err)
		return
	}
	if !removed {
		httpx.Error(w, http.StatusNotFound, "Entry not found in watchlist")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Removed from watchlist"})
}

// ── Readlist ──────────────────────────────────────────────────────────────────

// GET /api/users/me/readlist?status=reading
func (h *Handler) myReadlist(w http.ResponseWriter, r *http.Request) {
	h.respondReadlist(w, r, middleware.UserFrom(r).UserID)
}

// GET /api/users/{username}/readlist — public
func (h *Handler) publicReadlist(w http.ResponseWriter, r *http.Request) {
	target, err := h.repo.UserByUsername(r.Context(), chi.URLParam(r, "username"))
	if err != nil {
		notFoundOr(w, err, "User not found", "fetch readlist")
		return
	}
	h.respondReadlist(w, r, target.ID)
}

func (h *Handler) respondReadlist(w http.ResponseWriter, r *http.Request, userID int) {
	entries, err := h.repo.Readlist(r.Context(), userID, r.URL.Query().Get("status"))
	if err != nil {
		httpx.Internal(w, "fetch readlist", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": entries})
}

// GET /api/users/me/readlist/{mangaId}
func (h *Handler) myReadlistEntry(w http.ResponseWriter, r *http.Request) {
	mangaID, ok := httpx.IntParam(chi.URLParam(r, "mangaId"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid manga ID")
		return
	}
	entry, err := h.repo.ReadlistEntry(r.Context(), middleware.UserFrom(r).UserID, mangaID)
	if err != nil {
		notFoundOr(w, err, "Not in readlist", "fetch readlist entry")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": entry})
}

// POST /api/users/me/readlist
func (h *Handler) upsertReadlist(w http.ResponseWriter, r *http.Request) {
	var body listBody
	if err := httpx.Decode(r, &body); err != nil || body.MangaID == nil {
		httpx.Error(w, http.StatusBadRequest, "mangaId is required")
		return
	}
	if !slices.Contains(readStatuses, body.Status) {
		httpx.Error(w, http.StatusBadRequest, "status must be one of: "+strings.Join(readStatuses, ", "))
		return
	}
	if !body.validScore(w) {
		return
	}
	entry, err := h.repo.UpsertReadlist(r.Context(), middleware.UserFrom(r).UserID, *body.MangaID, repo.ListUpsert{
		Status: body.Status, Score: body.Score, Progress: body.ChaptersRead,
		Volumes: body.VolumesRead, Notes: body.Notes,
	})
	if err != nil {
		notFoundOr(w, err, err.Error(), "update readlist")
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"data": entry, "message": "Readlist updated"})
}

// PUT /api/users/me/readlist/{mangaId}
func (h *Handler) updateReadlist(w http.ResponseWriter, r *http.Request) {
	mangaID, ok := httpx.IntParam(chi.URLParam(r, "mangaId"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid manga ID")
		return
	}
	var body listBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid input data")
		return
	}
	if body.Status == "" {
		body.Status = "reading"
	} else if !slices.Contains(readStatuses, body.Status) {
		httpx.Error(w, http.StatusBadRequest, "status must be one of: "+strings.Join(readStatuses, ", "))
		return
	}
	if !body.validScore(w) {
		return
	}
	entry, err := h.repo.UpsertReadlist(r.Context(), middleware.UserFrom(r).UserID, mangaID, repo.ListUpsert{
		Status: body.Status, Score: body.Score, Progress: body.ChaptersRead,
		Volumes: body.VolumesRead, Notes: body.Notes,
	})
	if err != nil {
		notFoundOr(w, err, err.Error(), "update readlist entry")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": entry, "message": "Readlist updated"})
}

// DELETE /api/users/me/readlist/{mangaId}
func (h *Handler) removeReadlist(w http.ResponseWriter, r *http.Request) {
	mangaID, ok := httpx.IntParam(chi.URLParam(r, "mangaId"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid manga ID")
		return
	}
	removed, err := h.repo.RemoveReadlist(r.Context(), middleware.UserFrom(r).UserID, mangaID)
	if err != nil {
		httpx.Internal(w, "remove from readlist", err)
		return
	}
	if !removed {
		httpx.Error(w, http.StatusNotFound, "Entry not found in readlist")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Removed from readlist"})
}
