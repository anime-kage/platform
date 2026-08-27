package handler

// The manga release variant. Unlike anime, there is no in-app
// editor: page work (cleaning, typesetting) happens in real image tools, so a
// manga release is "bring your own pages" — finished RO page images plus the
// optional EN originals for side-by-side verification. It lands in review at
// creation, verify is a page flipper over the staged files, and publish
// copies the pages into permanent storage (R2 or local) as chapter_pages.

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"animekage/backend/internal/httpx"
	"animekage/backend/internal/middleware"
	"animekage/backend/internal/model"
	"animekage/backend/internal/repo"
)

// badPageError marks a page upload rejected for not being an image — the one
// staging failure that is the client's fault, not ours.
type badPageError struct{ filename string }

func (e badPageError) Error() string { return "not an image: " + e.filename }

// saveStagingPagesDir sniffs, orders and writes page images into dir as
// zero-padded NNN.ext files (stagingPages reads them back in that order).
func saveStagingPagesDir(dir string, files []*multipart.FileHeader) error {
	sorted := append([]*multipart.FileHeader{}, files...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Filename < sorted[j].Filename })
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for i, fh := range sorted {
		f, err := fh.Open()
		if err != nil {
			return err
		}
		data, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			return err
		}
		ext, ok := avatarExt[http.DetectContentType(data)]
		if !ok {
			return badPageError{fh.Filename}
		}
		name := fmt.Sprintf("%03d.%s", i+1, ext)
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// createMangaRelease handles POST /api/releases with medium=manga (the form
// is already parsed by createRelease). Fields: mangaId OR proposedTitle,
// chapterNumber, pages (RO images, required) + enPages, verifierId.
func (h *Handler) createMangaRelease(w http.ResponseWriter, r *http.Request) {
	var mangaID *int
	if v := r.FormValue("mangaId"); v != "" {
		id, err := strconv.Atoi(v)
		if err != nil || id < 1 {
			httpx.Error(w, http.StatusBadRequest, "Invalid mangaId")
			return
		}
		mangaID = &id
	}
	var proposedTitle *string
	if v := strings.TrimSpace(r.FormValue("proposedTitle")); v != "" {
		proposedTitle = &v
	}
	chapNum, err := strconv.ParseFloat(r.FormValue("chapterNumber"), 64)
	if (mangaID == nil && proposedTitle == nil) || err != nil || chapNum <= 0 {
		httpx.Error(w, http.StatusBadRequest, "chapterNumber and either mangaId or proposedTitle are required")
		return
	}

	verifierID, ok := h.parseVerifierField(w, r)
	if !ok {
		return
	}

	roPages := r.MultipartForm.File["pages"]
	if len(roPages) == 0 {
		httpx.Error(w, http.StatusBadRequest, "Paginile traduse sunt obligatorii (câmpul 'pages')")
		return
	}
	enPages := r.MultipartForm.File["enPages"]
	if len(roPages) > 500 || len(enPages) > 500 {
		httpx.Error(w, http.StatusBadRequest, "Too many pages (max 500)")
		return
	}

	rel, err := h.repo.CreateRelease(r.Context(), repo.CreateReleaseInput{
		Medium: "manga", MangaID: mangaID, ProposedTitle: proposedTitle,
		ChapterNumber: &chapNum, UploaderID: middleware.UserFrom(r).UserID, VerifierID: verifierID,
	})
	if err != nil {
		if repo.IsForeignKeyViolation(err) {
			httpx.Error(w, http.StatusNotFound, "Manga not found")
			return
		}
		httpx.Internal(w, "create release", err)
		return
	}
	// from here on, failures must tear the half-made release down again
	teardown := func() {
		_ = os.RemoveAll(filepath.Join(h.cfg.StagingDir, strconv.Itoa(rel.ID)))
		if _, derr := h.repo.DeleteRelease(r.Context(), rel.ID); derr != nil {
			slog.Error("orphaned release after failed create", "id", rel.ID, "err", derr)
		}
	}
	savePages := func(lang string, files []*multipart.FileHeader) bool {
		if len(files) == 0 {
			return true
		}
		dir := filepath.Join(h.cfg.StagingDir, strconv.Itoa(rel.ID), "pages", lang)
		if err := saveStagingPagesDir(dir, files); err != nil {
			teardown()
			var bad badPageError
			if errors.As(err, &bad) {
				httpx.Error(w, http.StatusBadRequest, "Doar imagini JPEG, PNG, WebP sau GIF: "+bad.filename)
			} else {
				httpx.Internal(w, "save staging pages", err)
			}
			return false
		}
		return true
	}
	if !savePages("ro", roPages) || !savePages("en", enPages) {
		return
	}

	stagingPath := strconv.Itoa(rel.ID) + "/pages"
	if err := h.repo.SetReleaseStagingPath(r.Context(), rel.ID, &stagingPath); err != nil {
		teardown()
		httpx.Internal(w, "save staging path", err)
		return
	}

	// pages arrive finished — straight to the verify queue
	if err := h.repo.SetReleaseState(r.Context(), rel.ID, []string{"draft"}, "in_review", nil, nil); err != nil {
		teardown()
		httpx.Internal(w, "submit release", err)
		return
	}

	if verifierID != nil {
		if err := h.repo.SetLastVerifier(r.Context(), middleware.UserFrom(r).UserID, *verifierID); err != nil {
			slog.Warn("save last verifier", "userId", middleware.UserFrom(r).UserID, "err", err)
		}
	}

	rel, err = h.repo.ReleaseByID(r.Context(), rel.ID)
	if err != nil {
		httpx.Internal(w, "fetch release", err)
		return
	}
	h.deriveRelease(rel)
	httpx.JSON(w, http.StatusCreated, map[string]any{"data": rel})
}

// GET /api/releases/{id}/page/{lang}/{idx} — one staged page for the verify
// flipper. idx is 1-based (pages are named 001.ext onward).
func (h *Handler) releasePage(w http.ResponseWriter, r *http.Request) {
	rel := h.loadRelease(w, r)
	if rel == nil {
		return
	}
	lang := chi.URLParam(r, "lang")
	idx, ok := httpx.IntParam(chi.URLParam(r, "idx"))
	if (lang != "ro" && lang != "en") || !ok || idx < 1 {
		httpx.Error(w, http.StatusBadRequest, "Invalid page reference")
		return
	}
	names := h.stagingPages(rel.ID, lang)
	if idx > len(names) {
		httpx.Error(w, http.StatusNotFound, "Page not found")
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=300")
	http.ServeFile(w, r, filepath.Join(h.cfg.StagingDir, strconv.Itoa(rel.ID), "pages", lang, names[idx-1]))
}

// POST /api/releases/{id}/pages — the uploader replaces one edition after a
// request-changes verdict (multipart: lang + pages). Writes into a temp dir
// first so a rejected upload can't destroy the current pages.
func (h *Handler) reuploadReleasePages(w http.ResponseWriter, r *http.Request) {
	rel := h.loadRelease(w, r)
	if rel == nil {
		return
	}
	if rel.Medium != "manga" {
		httpx.Error(w, http.StatusBadRequest, "Not a manga release")
		return
	}
	if rel.UploaderID != middleware.UserFrom(r).UserID {
		httpx.Error(w, http.StatusForbidden, "Only the uploader can replace pages")
		return
	}
	if rel.State != "draft" && rel.State != "changes_requested" {
		httpx.Error(w, http.StatusConflict, "Pages can only be replaced in draft or changes_requested (current state: '"+rel.State+"')")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 300<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Upload prea mare (max 300MB)")
		return
	}
	defer r.MultipartForm.RemoveAll()

	lang := r.FormValue("lang")
	if lang == "" {
		lang = "ro"
	}
	if lang != "ro" && lang != "en" {
		httpx.Error(w, http.StatusBadRequest, "lang must be 'ro' or 'en'")
		return
	}
	files := r.MultipartForm.File["pages"]
	if len(files) == 0 {
		httpx.Error(w, http.StatusBadRequest, "Nicio pagină încărcată (câmpul 'pages')")
		return
	}
	if len(files) > 500 {
		httpx.Error(w, http.StatusBadRequest, "Too many pages (max 500)")
		return
	}

	dir := filepath.Join(h.cfg.StagingDir, strconv.Itoa(rel.ID), "pages", lang)
	tmp := dir + ".tmp"
	_ = os.RemoveAll(tmp)
	if err := saveStagingPagesDir(tmp, files); err != nil {
		_ = os.RemoveAll(tmp)
		var bad badPageError
		if errors.As(err, &bad) {
			httpx.Error(w, http.StatusBadRequest, "Doar imagini JPEG, PNG, WebP sau GIF: "+bad.filename)
		} else {
			httpx.Internal(w, "save staging pages", err)
		}
		return
	}
	if err := os.RemoveAll(dir); err != nil {
		httpx.Internal(w, "replace staging pages", err)
		return
	}
	if err := os.Rename(tmp, dir); err != nil {
		httpx.Internal(w, "replace staging pages", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"message": "Pagini înlocuite", "language": lang, "count": len(files),
	})
}

// publishMangaRelease is publishRelease's manga arm (state already checked).
// Body (optional unless the release has no catalog series): {mangaId,
// chapterNumber}. Chapter find-or-create → staged pages copied to permanent
// storage (R2 when configured) as chapter_pages → published → staging freed.
func (h *Handler) publishMangaRelease(w http.ResponseWriter, r *http.Request, rel *model.Release) {
	var body struct {
		MangaID       *int     `json:"mangaId"`
		ChapterNumber *float64 `json:"chapterNumber"`
	}
	if err := httpx.Decode(r, &body); err != nil && !errors.Is(err, io.EOF) {
		httpx.Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	mangaID := rel.MangaID
	if body.MangaID != nil {
		mangaID = body.MangaID
	}
	if mangaID == nil {
		title := ""
		if rel.ProposedTitle != nil {
			title = " („" + *rel.ProposedTitle + "”)"
		}
		httpx.Error(w, http.StatusBadRequest, "Release-ul nu are încă o serie în catalog"+title+" — importă titlul din MAL și alege-l la publicare")
		return
	}
	chapNum := 0.0
	if rel.ChapterNumber != nil {
		chapNum = *rel.ChapterNumber
	}
	if body.ChapterNumber != nil {
		chapNum = *body.ChapterNumber
	}
	if chapNum <= 0 {
		httpx.Error(w, http.StatusBadRequest, "Invalid chapterNumber")
		return
	}

	roNames := h.stagingPages(rel.ID, "ro")
	if len(roNames) == 0 {
		httpx.Error(w, http.StatusConflict, "Staging pages no longer exist")
		return
	}

	// pin the confirmed mapping on the release (clears any proposed title)
	if rel.MangaID == nil || *mangaID != *rel.MangaID ||
		rel.ChapterNumber == nil || chapNum != *rel.ChapterNumber {
		if err := h.repo.SetReleaseTargetManga(r.Context(), rel.ID, *mangaID, chapNum); err != nil {
			if repo.IsForeignKeyViolation(err) {
				httpx.Error(w, http.StatusNotFound, "Manga not found")
				return
			}
			httpx.Internal(w, "retarget release", err)
			return
		}
	}

	chapLabel := strconv.FormatFloat(chapNum, 'f', -1, 64)
	ch, err := h.repo.ChapterByNumber(r.Context(), *mangaID, chapNum)
	if errors.Is(err, repo.ErrNotFound) {
		title := "Capitolul " + chapLabel
		ch, err = h.repo.CreateChapter(r.Context(), *mangaID, chapNum, repo.ChapterInput{Title: &title})
		if errors.Is(err, repo.ErrExists) { // lost a race — someone made it first
			ch, err = h.repo.ChapterByNumber(r.Context(), *mangaID, chapNum)
		}
	}
	if err != nil {
		httpx.Internal(w, "resolve chapter", err)
		return
	}

	ref := &repo.ChapterRef{MangaID: *mangaID, ChapterNumber: chapNum}
	for _, lang := range []string{"ro", "en"} {
		names := h.stagingPages(rel.ID, lang)
		if len(names) == 0 {
			continue
		}
		urls := make([]string, 0, len(names))
		for i, name := range names {
			data, err := os.ReadFile(filepath.Join(h.cfg.StagingDir, strconv.Itoa(rel.ID), "pages", lang, name))
			if err != nil {
				httpx.Internal(w, "read staging page", err)
				return
			}
			url, err := h.storePageImage(r.Context(), ref, ch.ID, lang, i, http.DetectContentType(data), data)
			if err != nil {
				httpx.Internal(w, "publish page", err)
				return
			}
			urls = append(urls, url)
		}
		if err := h.repo.ReplaceChapterPages(r.Context(), ch.ID, lang, urls); err != nil {
			httpx.Internal(w, "publish pages", err)
			return
		}
	}

	publisher := middleware.UserFrom(r).UserID
	if err := h.repo.SetReleaseState(r.Context(), rel.ID, []string{"approved"}, "published", nil, nil, publisher); err != nil {
		notFoundOr(w, err, "Release is no longer approved", "mark published")
		return
	}
	// pages now live in permanent storage — staging is done its job
	if err := h.repo.SetReleaseStagingPath(r.Context(), rel.ID, nil); err != nil {
		slog.Warn("clear staging path", "release", rel.ID, "err", err)
	}
	h.removeStaging(rel.ID)

	httpx.JSON(w, http.StatusOK, map[string]any{
		"message":   "Publicat — capitolul e live cu paginile proprii",
		"chapterId": ch.ID,
		"mangaId":   *mangaID,
	})
}
