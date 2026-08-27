package handler

// Site announcements — the "Știri & anunțuri" strip on /home. Reading is open
// to any member; writing is admin/moderator, same bar as pinning a thread.

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"animekage/backend/internal/httpx"
	"animekage/backend/internal/middleware"
	"animekage/backend/internal/model"
	"animekage/backend/internal/repo"
	"animekage/backend/internal/slug"
)

const (
	announcementTagMax   = 24
	announcementTitleMax = 160
	announcementBodyMax  = 20000
	announcementListMax  = 50
)

// GET /api/announcements?limit=&drafts=1
// `drafts=1` is admin/moderator only — a member asking for it just gets the
// published feed rather than a 403, because the flag is a UI convenience and
// not a resource of its own.
func (h *Handler) listAnnouncements(w http.ResponseWriter, r *http.Request) {
	limit := httpx.QueryInt(r, "limit", 10, 1, announcementListMax)
	drafts := r.URL.Query().Get("drafts") == "1" && canEditAnnouncements(r)
	rows, err := h.repo.ListAnnouncements(r.Context(), drafts, limit)
	if err != nil {
		httpx.Internal(w, "list announcements", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": rows})
}

// POST /api/announcements
func (h *Handler) createAnnouncement(w http.ResponseWriter, r *http.Request) {
	in, ok := decodeAnnouncement(w, r)
	if !ok {
		return
	}
	a, err := h.repo.CreateAnnouncement(r.Context(), middleware.UserFrom(r).UserID,
		in.tag, in.title, in.body, in.url, in.cover, in.published)
	if err != nil {
		httpx.Internal(w, "create announcement", err)
		return
	}
	// Minted once, here, and never regenerated on edit: the slug is the URL
	// people paste, so a retitled post keeps the link it went out with.
	h.assignAnnouncementSlug(r, &a)
	httpx.JSON(w, http.StatusCreated, map[string]any{"data": a})
}

// PUT /api/announcements/{id}
func (h *Handler) updateAnnouncement(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid announcement ID")
		return
	}
	in, ok := decodeAnnouncement(w, r)
	if !ok {
		return
	}
	a, err := h.repo.UpdateAnnouncement(r.Context(), id, in.tag, in.title, in.body, in.url, in.cover, in.published)
	if err != nil {
		notFoundOr(w, err, "Anunțul nu există", "update announcement")
		return
	}
	// A post written before slugs existed, or one whose title was empty at
	// creation, gets one the first time it is saved.
	if a.Slug == nil {
		h.assignAnnouncementSlug(r, &a)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": a})
}

// GET /api/announcements/{idOrSlug} — one post, for its own page.
func (h *Handler) getAnnouncement(w http.ResponseWriter, r *http.Request) {
	param := chi.URLParam(r, "id")
	var a model.Announcement
	err := repo.ErrNotFound
	if id, ok := httpx.IntParam(param); ok {
		a, err = h.repo.AnnouncementByID(r.Context(), id)
	}
	if err != nil {
		a, err = h.repo.AnnouncementBySlug(r.Context(), param)
	}
	if err != nil {
		notFoundOr(w, err, "Anunțul nu există", "get announcement")
		return
	}
	// A draft is visible only to the people who can edit it.
	if !a.IsPublished && !canEditAnnouncements(r) {
		httpx.Error(w, http.StatusNotFound, "Anunțul nu există")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": a})
}

// POST /api/announcements/image — cover and in-body images, admin/moderator.
//
// A far higher ceiling than a poster gets: these are full-width screenshots and
// photos, not 240px covers.
func (h *Handler) uploadAnnouncementImage(w http.ResponseWriter, r *http.Request) {
	h.uploadImage(w, r, "announcements", postMaxUpload, nil)
}

// assignAnnouncementSlug gives a post its URL segment, disambiguating with the
// id when two posts share a title. Best-effort: a post with no slug still opens
// at its numeric id, so a failure here is cosmetic.
func (h *Handler) assignAnnouncementSlug(r *http.Request, a *model.Announcement) {
	base := slug.Make(a.Title)
	if base == "" {
		return
	}
	if err := h.repo.SetAnnouncementSlug(r.Context(), a.ID, base); err != nil {
		base = fmt.Sprintf("%s-%d", base, a.ID)
		if err := h.repo.SetAnnouncementSlug(r.Context(), a.ID, base); err != nil {
			return
		}
	}
	a.Slug = &base
}

// DELETE /api/announcements/{id}
func (h *Handler) deleteAnnouncement(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid announcement ID")
		return
	}
	if err := h.repo.DeleteAnnouncement(r.Context(), id); err != nil {
		notFoundOr(w, err, "Anunțul nu există", "delete announcement")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"message": "Anunț șters"})
}

// announcementInput is the validated body shared by create and update.
type announcementInput struct {
	tag, title string
	body, url  *string
	cover      *string
	published  bool
}

func decodeAnnouncement(w http.ResponseWriter, r *http.Request) (announcementInput, bool) {
	var body struct {
		Tag         string `json:"tag"`
		Title       string `json:"title"`
		Body        string `json:"body"`
		URL         string `json:"url"`
		CoverURL    string `json:"coverUrl"`
		IsPublished *bool  `json:"isPublished"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid input data")
		return announcementInput{}, false
	}

	in := announcementInput{
		tag:       strings.TrimSpace(body.Tag),
		title:     strings.TrimSpace(body.Title),
		published: body.IsPublished == nil || *body.IsPublished, // default: goes out
	}
	if n := len([]rune(in.tag)); n < 2 || n > announcementTagMax {
		httpx.Error(w, http.StatusBadRequest, "Eticheta trebuie să aibă între 2 și 24 de caractere")
		return announcementInput{}, false
	}
	if n := len([]rune(in.title)); n < 3 || n > announcementTitleMax {
		httpx.Error(w, http.StatusBadRequest, "Titlul trebuie să aibă între 3 și 160 de caractere")
		return announcementInput{}, false
	}
	if text := strings.TrimSpace(body.Body); text != "" {
		if len([]rune(text)) > announcementBodyMax {
			httpx.Error(w, http.StatusBadRequest, "Textul e prea lung (max 20000 de caractere)")
			return announcementInput{}, false
		}
		in.body = &text
	}
	if link := strings.TrimSpace(body.URL); link != "" {
		// An announcement is rendered as an anchor, so a `javascript:` or `data:`
		// href here would be stored XSS aimed at every member's front page.
		// Internal path or plain https, nothing else.
		if !strings.HasPrefix(link, "/") && !strings.HasPrefix(link, "https://") {
			httpx.Error(w, http.StatusBadRequest, "Linkul trebuie să înceapă cu / sau https://")
			return announcementInput{}, false
		}
		if strings.HasPrefix(link, "//") {
			httpx.Error(w, http.StatusBadRequest, "Linkul trebuie să înceapă cu / sau https://")
			return announcementInput{}, false
		}
		in.url = &link
	}
	// The cover is a path we minted ourselves via POST /image, so it must be a
	// local upload path — never an arbitrary remote URL pasted into the field.
	if c := strings.TrimSpace(body.CoverURL); c != "" {
		if !strings.HasPrefix(c, "/uploads/") {
			httpx.Error(w, http.StatusBadRequest, "Imaginea trebuie încărcată, nu lipită ca link")
			return announcementInput{}, false
		}
		in.cover = &c
	}
	return in, true
}

// canEditAnnouncements mirrors the role gate the write routes carry, for the
// one place that needs to know it inside a read handler.
func canEditAnnouncements(r *http.Request) bool {
	u := middleware.UserFrom(r)
	if u == nil {
		return false
	}
	return u.Role == "admin" || u.Role == "moderator"
}
