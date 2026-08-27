package handler

// Subtitle endpoints. These manage our own tracks; the player gets
// them via the stream endpoint, which merges published .vtt rows into the
// resolved stream. The Phase 4 release pipeline will write these rows itself —
// this is the manual/admin path.

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"animekage/backend/internal/httpx"
	"animekage/backend/internal/middleware"
	"animekage/backend/internal/model"
	"animekage/backend/internal/repo"
	"animekage/backend/internal/subs"
)

// GET /api/episodes/{id}/subtitles — published tracks, RO first.
func (h *Handler) listSubtitles(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid episode ID")
		return
	}
	subs, err := h.repo.PublishedSubtitles(r.Context(), id)
	if err != nil {
		httpx.Internal(w, "fetch subtitles", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": subs})
}

type subtitleBody struct {
	Language string  `json:"language"`
	Label    *string `json:"label"`
	Format   string  `json:"format"` // default 'vtt'
	URL      string  `json:"url"`
}

// maxSubtitleUpload caps a hand-attached track. Subtitles are text; a file this
// size is a mistake or an attack, not a subtitle.
const maxSubtitleUpload = 5 << 20

// POST /api/episodes/{id}/subtitles/upload  (content role) — multipart.
//
// Attaches a subtitle file directly to an episode, for the case the release
// pipeline does not cover: an episode whose source was linked by hand and whose
// translation was done elsewhere (or predates this database).
//
// The sibling endpoint above takes a URL to a track hosted somewhere else. This
// one takes the file, and that difference matters: it converts on the way in.
// mergeOwnSubtitles skips any row whose format is not vtt, because <track>
// renders nothing else — so an .srt or .ass stored as-is would be accepted here,
// listed in the admin UI, and then silently never appear in the player. Parsing
// to cues and writing WebVTT is what makes the upload actually mean something.
//
// Written exactly where the publish pipeline writes its own tracks
// (uploads/subs/ep-{id}-{lang}.vtt, see writeTrack), and upserted rather than
// inserted, so re-uploading a corrected file replaces the track instead of
// colliding with it.
//
// Note what this cannot fix: a source played through an <iframe> can never show
// our track, whatever is attached here. Same-origin. It takes effect only for
// sources resolved into our own player.
func (h *Handler) uploadSubtitle(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid episode ID")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxSubtitleUpload+4096)
	if err := r.ParseMultipartForm(maxSubtitleUpload); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Fișierul de subtitrare trebuie să fie sub 5 MB.")
		return
	}
	defer r.MultipartForm.RemoveAll()

	lang := strings.ToLower(strings.TrimSpace(r.FormValue("language")))
	if lang == "" {
		lang = "ro"
	}
	if len(lang) > 10 {
		httpx.Error(w, http.StatusBadRequest, "Invalid language")
		return
	}

	file, hdr, err := r.FormFile("file")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "Atașează un fișier .srt, .vtt sau .ass")
		return
	}
	defer file.Close()

	raw, err := io.ReadAll(file)
	if err != nil {
		httpx.Internal(w, "read subtitle upload", err)
		return
	}
	// Parse dispatches on the extension and understands SubRip, WebVTT and ASS.
	cues, err := subs.Parse(hdr.Filename, raw)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "Subtitrare invalidă: "+err.Error())
		return
	}
	if len(cues) == 0 {
		httpx.Error(w, http.StatusBadRequest, "Fișierul nu conține nicio replică.")
		return
	}

	dir := filepath.Join(h.cfg.UploadsDir, "subs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		httpx.Internal(w, "create subs dir", err)
		return
	}
	name := fmt.Sprintf("ep-%d-%s.vtt", id, lang)
	if err := os.WriteFile(filepath.Join(dir, name), []byte(subs.WriteVTT(cues)), 0o644); err != nil {
		httpx.Internal(w, "write subtitle vtt", err)
		return
	}

	var label *string
	if v := strings.TrimSpace(r.FormValue("label")); v != "" {
		label = &v
	}
	uploader := middleware.UserFrom(r).UserID
	// source_sub records provenance: this track came from a person with a file,
	// not from a release, which is worth knowing when auditing a line later.
	source := fmt.Sprintf("manual:%d", uploader)
	sub, err := h.repo.UpsertPublishedSubtitle(r.Context(), repo.SubtitleInput{
		EpisodeID: id, Language: lang, Label: label, Format: "vtt",
		URL: "/uploads/subs/" + name, TranslatorID: &uploader, SourceSub: &source,
	})
	if err != nil {
		if repo.IsForeignKeyViolation(err) {
			httpx.Error(w, http.StatusNotFound, "Episode not found")
			return
		}
		httpx.Internal(w, "upsert subtitle", err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"data": sub, "cues": len(cues)})
}

// POST /api/episodes/{id}/subtitles  (admin/translator)
func (h *Handler) addSubtitle(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid episode ID")
		return
	}
	var body subtitleBody
	if err := httpx.Decode(r, &body); err != nil || body.URL == "" || body.Language == "" {
		httpx.Error(w, http.StatusBadRequest, "language and url are required")
		return
	}
	if err := validateHostingURL(body.URL, h.cfg.ContentHosts); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Format == "" {
		body.Format = "vtt"
	}
	if body.Format != "vtt" && body.Format != "srt" && body.Format != "ass" {
		httpx.Error(w, http.StatusBadRequest, "format must be 'vtt', 'srt' or 'ass'")
		return
	}
	sub, err := h.repo.AddSubtitle(r.Context(), repo.SubtitleInput{
		EpisodeID: id, Language: body.Language, Label: body.Label,
		Format: body.Format, URL: body.URL, Status: "published",
	})
	if errors.Is(err, repo.ErrExists) {
		httpx.Error(w, http.StatusConflict, "A published subtitle for this language already exists")
		return
	}
	if err != nil {
		httpx.Internal(w, "add subtitle", err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"data": sub})
}

// GET /api/episodes/{id}/subtitles.srt?lang=ro  (content role) — the
// episode's published subtitle, downloaded as SubRip. We store the published
// track as .vtt at publish time, so this converts VTT → SRT on the way out.
func (h *Handler) episodeSubtitleSRT(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid episode ID")
		return
	}
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = "ro"
	}

	tracks, err := h.repo.PublishedSubtitles(r.Context(), id)
	if err != nil {
		httpx.Internal(w, "fetch subtitles", err)
		return
	}
	var sub *model.Subtitle
	for i := range tracks {
		if tracks[i].Language == lang {
			sub = &tracks[i]
			break
		}
	}
	if sub == nil {
		httpx.Error(w, http.StatusNotFound, "Nu există subtitrare „"+lang+"” publicată pentru acest episod")
		return
	}

	// only our own uploaded tracks live on disk and can be converted here
	relPath := strings.TrimPrefix(sub.URL, "/uploads/")
	if relPath == sub.URL {
		httpx.Error(w, http.StatusBadRequest, "Subtitrarea e găzduită extern — deschide direct: "+sub.URL)
		return
	}
	data, err := os.ReadFile(filepath.Join(h.cfg.UploadsDir, filepath.FromSlash(relPath)))
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "Fișierul subtitrării nu a fost găsit pe disc")
		return
	}
	var events []subs.Event
	switch sub.Format {
	case "srt":
		events, err = subs.ParseSRT(data)
	case "ass", "ssa":
		events, err = subs.ParseASS(data)
	default: // vtt
		events, err = subs.ParseVTT(data)
	}
	if err != nil {
		httpx.Internal(w, "parse subtitle", err)
		return
	}

	name := fmt.Sprintf("ep-%d", id)
	if title, num, lerr := h.repo.EpisodeLabel(r.Context(), id); lerr == nil {
		if slug := asciiSlug(title); slug != "" {
			name = fmt.Sprintf("%s-ep-%02d", slug, num)
		}
	}
	w.Header().Set("Content-Type", "application/x-subrip; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.%s.srt"`, name, lang))
	_, _ = io.WriteString(w, subs.WriteSRT(events))
}

// DELETE /api/subtitles/{id}  (admin/translator)
func (h *Handler) deleteSubtitle(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid subtitle ID")
		return
	}
	if err := h.repo.DeleteSubtitle(r.Context(), id); err != nil {
		notFoundOr(w, err, "Subtitle not found", "delete subtitle")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Subtitle deleted successfully"})
}
