package handler

// Release pipeline endpoints.
//
// A translator uploads video + EN sub into OUR staging (instant native
// preview; hosts re-encode for an hour). The sub is parsed into
// subtitle_events rows; the /translate editor (4.2) autosaves ro_text per
// row; submit → the /verify queue; approve/request-changes is the review
// gate. Uploading a finished RO sub ("bring your own sub", the Aegisub path)
// skips the editor and lands straight in review. Host uploads happen at
// approval (4.5), never before — rejected work must not burn host quota.

import (
	"context"
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
	"time"

	"github.com/go-chi/chi/v5"

	"animekage/backend/internal/httpx"
	"animekage/backend/internal/middleware"
	"animekage/backend/internal/model"
	"animekage/backend/internal/repo"
	"animekage/backend/internal/subs"
)

const (
	maxVideoBytes = 3 << 30 // 3 GB — a BD episode remux tops out well under this
	maxSubBytes   = 5 << 20
)

var videoExts = map[string]bool{".mp4": true, ".webm": true, ".mkv": true, ".m4v": true}

// canReview is the release-visibility gate: verifier-class roles plus the
// coordinator (who needs to see the whole pipeline to publish from it).
func canReview(r *http.Request) bool {
	u := middleware.UserFrom(r)
	return u != nil && (u.Role == "verifier" || u.Role == "coordinator" || u.Role == "moderator" || u.Role == "admin")
}

// canPublish is the publish gate: coordinators and admins only.
func canPublish(r *http.Request) bool {
	u := middleware.UserFrom(r)
	return u != nil && (u.Role == "coordinator" || u.Role == "admin")
}

// verdictAllowed enforces the assignment routing on approve/request-changes:
// a verifier may judge releases routed to them (or unrouted ones); everyone
// above verifier may judge anything. Writes the 403 itself.
func (h *Handler) verdictAllowed(w http.ResponseWriter, r *http.Request, rel *model.Release) bool {
	u := middleware.UserFrom(r)
	if u.Role == "verifier" && rel.AssignedVerifierID != nil && *rel.AssignedVerifierID != u.UserID {
		name := "altcuiva"
		if rel.AssignedVerifierName != nil {
			name = *rel.AssignedVerifierName
		}
		httpx.Error(w, http.StatusForbidden, "Verificarea e rutată către "+name+" — cere-i unui coordonator să o reatribuie")
		return false
	}
	return true
}

// loadRelease fetches the release and enforces visibility: the uploader and
// verifier-class roles only. Writes the error response itself on failure.
func (h *Handler) loadRelease(w http.ResponseWriter, r *http.Request) *model.Release {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid release ID")
		return nil
	}
	rel, err := h.repo.ReleaseByID(r.Context(), id)
	if err != nil {
		notFoundOr(w, err, "Release not found", "fetch release")
		return nil
	}
	if rel.UploaderID != middleware.UserFrom(r).UserID && !canReview(r) {
		httpx.Error(w, http.StatusForbidden, "Not your release")
		return nil
	}
	h.deriveRelease(rel)
	if v, ok := h.translating.Load(rel.ID); ok {
		rel.Translating = v.(*translationStatus).snapshot().Running
	}
	return rel
}

// deriveRelease fills the fields that live on disk, not in the DB.
func (h *Handler) deriveRelease(rel *model.Release) {
	if rel.Medium == "manga" {
		rel.PageCount = len(h.stagingPages(rel.ID, "ro"))
		rel.EnPageCount = len(h.stagingPages(rel.ID, "en"))
		return
	}
	// Either location counts: the preview and the downloads both work off
	// whichever one holds the video.
	rel.HasVideo = rel.StagingPath != nil || (rel.R2Key != nil && *rel.R2Key != "")
}

// stagingPages lists a manga release's staged page files for one edition,
// sorted by filename (they're written zero-padded, so this is page order).
func (h *Handler) stagingPages(releaseID int, lang string) []string {
	entries, err := os.ReadDir(filepath.Join(h.cfg.StagingDir, strconv.Itoa(releaseID), "pages", lang))
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

// parseVerifierField reads the optional verifierId form field (release
// routing). Writes the error response itself; ok=false means it did.
func (h *Handler) parseVerifierField(w http.ResponseWriter, r *http.Request) (verifierID *int, ok bool) {
	v := r.FormValue("verifierId")
	if v == "" {
		return nil, true
	}
	id, err := strconv.Atoi(v)
	if err != nil || id < 1 {
		httpx.Error(w, http.StatusBadRequest, "Invalid verifierId")
		return nil, false
	}
	if id == middleware.UserFrom(r).UserID {
		httpx.Error(w, http.StatusBadRequest, "Nu îți poți verifica propria traducere")
		return nil, false
	}
	if !h.isAssignableVerifier(r.Context(), id) {
		httpx.Error(w, http.StatusBadRequest, "Utilizatorul ales nu e verificator")
		return nil, false
	}
	return &id, true
}

// capExemptRoles never hit the in-flight cap. Coordinators and admins are the
// ones who drain the queue — locking them out of their own pipeline during a
// backlog is exactly the wrong failure mode.
var capExemptRoles = map[string]bool{"admin": true, "coordinator": true}

// releaseQuota reports how many unpublished anime releases the caller holds and
// whether they may start another. The per-user override on `users.release_cap`
// wins over the global TRANSLATOR_RELEASE_CAP; a 0 on either disables the cap.
//
// A lookup failure returns ok — a flaky count must not stop people working.
func (h *Handler) releaseQuota(r *http.Request) (used, limit int, exempt, ok bool) {
	limit = h.cfg.TranslatorReleaseCap
	u := middleware.UserFrom(r)
	if u == nil {
		return 0, limit, false, true
	}
	if capExemptRoles[u.Role] {
		return 0, limit, true, true
	}
	used, override, err := h.repo.UnpublishedAnimeReleases(r.Context(), u.UserID)
	if err != nil {
		slog.Error("count unpublished releases", "user", u.UserID, "err", err)
		return 0, limit, false, true
	}
	if override != nil {
		limit = *override
	}
	if limit <= 0 {
		return used, limit, true, true
	}
	return used, limit, false, used < limit
}

// GET /api/releases/quota — what the upload form shows before you pick a file.
func (h *Handler) releaseQuotaHandler(w http.ResponseWriter, r *http.Request) {
	used, limit, exempt, ok := h.releaseQuota(r)
	httpx.JSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"used": used, "limit": limit, "exempt": exempt, "canUpload": ok,
	}})
}

// POST /api/releases  (translator+, multipart)
// Anime (default): animeId OR proposedTitle, episodeNumber, video (file),
// sub (EN .srt/.ass/.vtt), roSub (finished RO sub — skips the editor,
// straight to review). At least one of sub/roSub is required.
// Manga (medium=manga): mangaId OR proposedTitle, chapterNumber, pages
// (finished RO page images) + optional enPages — always straight to review
//.
func (h *Handler) createRelease(w http.ResponseWriter, r *http.Request) {
	// Before touching the body: rejecting after reading a 3 GB upload would
	// waste the translator's entire transfer to tell them something we knew up
	// front. The medium therefore has to come from the query string — it lives
	// in the multipart body, and parsing that is the thing we are avoiding.
	// Absent means anime, which is the safe default: manga is never the reason
	// staging fills up.
	if r.URL.Query().Get("medium") != "manga" {
		if used, limit, _, ok := h.releaseQuota(r); !ok {
			httpx.Error(w, http.StatusConflict, fmt.Sprintf(
				"Ai %d din %d episoade neterminate. Așteaptă să fie verificat și publicat unul "+
					"înainte de a urca altul — spațiul de lucru de pe server e limitat.", used, limit))
			return
		}
	}
	// Same reasoning as the chunk endpoint (handler/uploads.go): the server's
	// 30s ReadTimeout caps reading the whole body, so a multipart carrying even
	// a modest video cannot finish on a domestic uplink. This path is the
	// pre-chunking one and now mostly serves small uploads and the integration
	// tests, but a 30s ceiling on it is still a trap.
	// Both deadlines, for the reason spelled out in handler/uploads.go: the write
	// timeout is measured from the request headers, so a slow body exhausts it
	// before the handler can reply and the client sees a 502 for an upload that
	// actually succeeded.
	relRC := http.NewResponseController(w)
	relDeadline := time.Now().Add(uploadReadBudget)
	if err := relRC.SetReadDeadline(relDeadline); err != nil {
		slog.Warn("could not extend read deadline for release upload", "err", err)
	}
	if err := relRC.SetWriteDeadline(relDeadline); err != nil {
		slog.Warn("could not extend write deadline for release upload", "err", err)
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxVideoBytes+2*maxSubBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid upload (video must be under 3GB)")
		return
	}
	defer r.MultipartForm.RemoveAll()

	if r.FormValue("medium") == "manga" {
		h.createMangaRelease(w, r)
		return
	}

	var animeID *int
	if v := r.FormValue("animeId"); v != "" {
		id, err := strconv.Atoi(v)
		if err != nil || id < 1 {
			httpx.Error(w, http.StatusBadRequest, "Invalid animeId")
			return
		}
		animeID = &id
	}
	var proposedTitle *string
	if v := strings.TrimSpace(r.FormValue("proposedTitle")); v != "" {
		proposedTitle = &v
	}
	epNum, err2 := strconv.Atoi(r.FormValue("episodeNumber"))
	if (animeID == nil && proposedTitle == nil) || err2 != nil || epNum < 1 {
		httpx.Error(w, http.StatusBadRequest, "episodeNumber and either animeId or proposedTitle are required")
		return
	}

	verifierID, ok := h.parseVerifierField(w, r)
	if !ok {
		return
	}

	// The video arrives one of two ways.
	//
	// videoObjectKey: the browser PUT it straight to R2 with a presigned URL, so
	// it never touched this server or Cloudflare's 100 MiB body limit. videoSize
	// is what the client declared, checked against the object's real length
	// before anything uses it — a browser killed mid-PUT leaves a short object.
	// The bytes then stay in the bucket for the life of the release.
	//
	// A `video` part: the original single-request path. Kept because it is the
	// simpler contract and still correct for anything under the edge limit — and
	// for calls that do not go through Cloudflare at all, like the integration
	// tests, which talk to the handler directly.
	r2Key := strings.TrimSpace(r.FormValue("videoObjectKey"))
	r2Size, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("videoSize")), 10, 64)
	var (
		video    multipart.File
		videoExt string
	)
	if r2Key != "" {
		videoExt = strings.ToLower(filepath.Ext(r2Key))
	} else {
		var videoHdr *multipart.FileHeader
		var ferr error
		video, videoHdr, ferr = r.FormFile("video")
		if ferr != nil {
			httpx.Error(w, http.StatusBadRequest, "A video file is required")
			return
		}
		defer video.Close()
		videoExt = strings.ToLower(filepath.Ext(videoHdr.Filename))
	}
	if !videoExts[videoExt] {
		httpx.Error(w, http.StatusBadRequest, "Video must be .mp4, .webm, .mkv or .m4v")
		return
	}

	enEvents, err := formSubEvents(r, "sub")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "EN subtitle: "+err.Error())
		return
	}
	roEvents, err := formSubEvents(r, "roSub")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "RO subtitle: "+err.Error())
		return
	}
	// No subtitle uploaded is no longer fatal here: if the video carries an
	// embedded soft-sub, we lift it out below.

	rel, err := h.repo.CreateRelease(r.Context(), repo.CreateReleaseInput{
		Medium: "anime", AnimeID: animeID, ProposedTitle: proposedTitle,
		EpisodeNumber: &epNum, UploaderID: middleware.UserFrom(r).UserID, VerifierID: verifierID,
	})
	if err != nil {
		if repo.IsForeignKeyViolation(err) {
			httpx.Error(w, http.StatusNotFound, "Anime not found")
			return
		}
		httpx.Internal(w, "create release", err)
		return
	}
	// from here on, failures must tear the half-made release down again
	fail := func(action string, err error) {
		_ = os.RemoveAll(filepath.Join(h.cfg.StagingDir, strconv.Itoa(rel.ID)))
		if _, derr := h.repo.DeleteRelease(r.Context(), rel.ID); derr != nil {
			slog.Error("orphaned release after failed create", "id", rel.ID, "err", derr)
		}
		httpx.Internal(w, action, err)
	}
	// failClient is fail's 4xx sibling: the user's input was the problem, so
	// report it plainly instead of a 500.
	failClient := func(status int, msg string) {
		_ = os.RemoveAll(filepath.Join(h.cfg.StagingDir, strconv.Itoa(rel.ID)))
		if _, derr := h.repo.DeleteRelease(r.Context(), rel.ID); derr != nil {
			slog.Error("orphaned release after client error", "id", rel.ID, "err", derr)
		}
		httpx.Error(w, status, msg)
	}

	dir := filepath.Join(h.cfg.StagingDir, strconv.Itoa(rel.ID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fail("create staging dir", err)
		return
	}

	videoPath := filepath.Join(dir, "video"+videoExt)
	// videoSrc is what ffmpeg reads below: a presigned URL when the video stays
	// in the bucket, a local path otherwise.
	videoSrc := videoPath
	if r2Key != "" {
		// Verified, not copied. The object stays in R2 for the life of the
		// release, so staging no longer has to hold every in-flight episode.
		if cerr := h.verifyR2Upload(r, r2Key, r2Size); cerr != nil {
			failClient(http.StatusBadRequest, "Încărcarea video nu s-a finalizat — reia trimiterea fișierului.")
			slog.Warn("verify R2 video", "release", rel.ID, "key", r2Key, "err", cerr)
			return
		}
		url, uerr := h.storage.PresignGet(r.Context(), r2Key, presignTTL)
		if uerr != nil {
			fail("presign uploaded video", uerr)
			return
		}
		videoSrc = url
	} else {
		dst, cerr := os.Create(videoPath)
		if cerr != nil {
			fail("save staging video", cerr)
			return
		}
		_, cerr = io.Copy(dst, video)
		if closeErr := dst.Close(); cerr == nil {
			cerr = closeErr
		}
		if cerr != nil {
			fail("save staging video", cerr)
			return
		}
	}
	if r2Key != "" {
		if err := h.repo.SetReleaseR2Key(r.Context(), rel.ID, &r2Key); err != nil {
			fail("save r2 key", err)
			return
		}
	} else {
		stagingPath := strconv.Itoa(rel.ID) + "/video" + videoExt
		if err := h.repo.SetReleaseStagingPath(r.Context(), rel.ID, &stagingPath); err != nil {
			fail("save staging path", err)
			return
		}
	}

	// No EN file? Lift an embedded soft-sub track out of the video so the
	// translator needn't demux it by hand. Only a real subtitle stream can be
	// read — hard-subbed video (or no track at all) still needs a file.
	//
	// This runs even when a finished RO sub was attached. It used to only run
	// when there was no subtitle at all, which meant a "bring your own RO sub"
	// upload silently threw the English away — and then there was nothing to
	// compare a suspect line against later. Keeping both is the point.
	autoSub := false
	if enEvents == nil {
		extracted, exErr := subs.ExtractEmbedded(r.Context(), videoSrc)
		if exErr != nil {
			slog.Warn("subtitle auto-extract failed", "release", rel.ID, "err", exErr)
		}
		switch {
		case len(extracted) > 0:
			enEvents = extracted
			autoSub = roEvents == nil // it's the translation source only if nothing else is
		case roEvents == nil:
			msg := "Atașează o subtitrare: EN (de tradus) sau RO finală."
			if subs.HasFFmpeg() {
				msg = "Niciun fișier de subtitrare, iar în video nu e nicio pistă de subtitrare de extras (posibil hard-sub)."
			}
			failClient(http.StatusBadRequest, msg)
			return
		}
		// else: a RO sub with no extractable EN — fine, just no source column
	}

	// Merge into event rows. With only an EN sub the RO column starts empty
	// (the editor's job). A RO sub is authoritative: its rows are the draft,
	// marked edited (human work, auto-translate must not touch them); EN text
	// is attached by index when the row counts line up, else left blank.
	events := make([]repo.EventInput, 0)
	switch {
	case roEvents != nil:
		// EN pairs by index, so it needs the same row count; a retimed or
		// re-split RO sub can't be lined up that way and keeps an empty
		// source column rather than a misaligned one.
		pairEN := enEvents != nil && len(enEvents) == len(roEvents)
		if enEvents != nil && !pairEN {
			slog.Warn("EN source dropped: row counts differ",
				"release", rel.ID, "en", len(enEvents), "ro", len(roEvents))
		}
		for _, e := range roEvents {
			ev := repo.EventInput{Idx: e.Idx, StartMs: e.StartMs, EndMs: e.EndMs, RoText: e.Text, Edited: true}
			if pairEN {
				ev.EnText = enEvents[e.Idx].Text
			}
			events = append(events, ev)
		}
	default:
		for _, e := range enEvents {
			events = append(events, repo.EventInput{Idx: e.Idx, StartMs: e.StartMs, EndMs: e.EndMs, EnText: e.Text})
		}
	}
	if err := h.repo.ReplaceEvents(r.Context(), rel.ID, events); err != nil {
		fail("save subtitle events", err)
		return
	}

	if roEvents != nil { // bring-your-own-sub: straight to the verify queue
		if err := h.repo.SetReleaseState(r.Context(), rel.ID, []string{"draft"}, "in_review", nil, nil); err != nil {
			fail("submit release", err)
			return
		}
	}

	// remember the explicit pick as the next default
	if verifierID != nil {
		if err := h.repo.SetLastVerifier(r.Context(), middleware.UserFrom(r).UserID, *verifierID); err != nil {
			slog.Warn("save last verifier", "userId", middleware.UserFrom(r).UserID, "err", err)
		}
	}

	// Queue the MKV→MP4 rewrap last, once the release is otherwise complete.
	//
	// Order matters: the EN track was lifted out of the Matroska above, while the
	// source still *was* Matroska. The rewrap drops subtitle streams, so running it
	// any earlier would throw away the very thing the translator came for.
	h.queueRemuxIfNeeded(r.Context(), rel.ID, videoExt)

	rel, err = h.repo.ReleaseByID(r.Context(), rel.ID)
	if err != nil {
		httpx.Internal(w, "fetch release", err)
		return
	}
	rel.HasVideo = true
	httpx.JSON(w, http.StatusCreated, map[string]any{"data": rel, "autoSub": autoSub})
}

// isAssignableVerifier reports whether id is on the assignable list
// (verifier/coordinator/admin, not banned).
func (h *Handler) isAssignableVerifier(ctx context.Context, id int) bool {
	opts, err := h.repo.Verifiers(ctx)
	if err != nil {
		return false
	}
	for _, o := range opts {
		if o.ID == id {
			return true
		}
	}
	return false
}

// GET /api/releases/verifiers  (team) — the assignable people plus the
// caller's default (their last explicit pick).
func (h *Handler) listVerifiers(w http.ResponseWriter, r *http.Request) {
	opts, err := h.repo.Verifiers(r.Context())
	if err != nil {
		httpx.Internal(w, "list verifiers", err)
		return
	}
	last, err := h.repo.LastVerifierID(r.Context(), middleware.UserFrom(r).UserID)
	if err != nil {
		httpx.Internal(w, "list verifiers", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": opts, "lastVerifierId": last})
}

// PUT /api/releases/{id}/verifier  — reroute a release. The uploader can
// reassign their own; coordinators/admins anyone's. Body: {verifierId} (null
// clears the routing, putting it in every verifier's queue).
func (h *Handler) assignVerifier(w http.ResponseWriter, r *http.Request) {
	rel := h.loadRelease(w, r)
	if rel == nil {
		return
	}
	u := middleware.UserFrom(r)
	if rel.UploaderID != u.UserID && !canPublish(r) {
		httpx.Error(w, http.StatusForbidden, "Doar traducătorul, coordonatorii sau adminii pot reruta verificarea")
		return
	}
	var body struct {
		VerifierID *int `json:"verifierId"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid input data")
		return
	}
	if body.VerifierID != nil {
		if *body.VerifierID == rel.UploaderID {
			httpx.Error(w, http.StatusBadRequest, "Traducătorul nu își poate verifica propriul release")
			return
		}
		if !h.isAssignableVerifier(r.Context(), *body.VerifierID) {
			httpx.Error(w, http.StatusBadRequest, "Utilizatorul ales nu e verificator")
			return
		}
	}
	if err := h.repo.AssignVerifier(r.Context(), rel.ID, body.VerifierID); err != nil {
		notFoundOr(w, err, "Release not found", "assign verifier")
		return
	}
	if body.VerifierID != nil && rel.UploaderID == u.UserID {
		if err := h.repo.SetLastVerifier(r.Context(), u.UserID, *body.VerifierID); err != nil {
			slog.Warn("save last verifier", "userId", u.UserID, "err", err)
		}
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Verificator actualizat"})
}

// releaseLabel is a short human name for a release, used in the translator's
// notifications: the linked series title (or the proposed one) plus the
// episode/chapter number when known. Falls back to a generic phrase so the
// sentence always reads.
func releaseLabel(rel *model.Release) string {
	name := "Release-ul tău"
	switch {
	case rel.AnimeTitle != nil && *rel.AnimeTitle != "":
		name = *rel.AnimeTitle
	case rel.MangaTitle != nil && *rel.MangaTitle != "":
		name = *rel.MangaTitle
	case rel.ProposedTitle != nil && *rel.ProposedTitle != "":
		name = *rel.ProposedTitle
	}
	switch {
	case rel.EpisodeNumber != nil:
		return fmt.Sprintf("%s ep. %d", name, *rel.EpisodeNumber)
	case rel.ChapterNumber != nil:
		return fmt.Sprintf("%s cap. %g", name, *rel.ChapterNumber)
	}
	return name
}

// formSubEvents reads an optional subtitle file field and parses it.
// (nil, nil) when the field is absent.
func formSubEvents(r *http.Request, field string) ([]subs.Event, error) {
	f, hdr, err := r.FormFile(field)
	if errors.Is(err, http.ErrMissingFile) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if hdr.Size > maxSubBytes {
		return nil, fmt.Errorf("file too large (max 5MB)")
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	return subs.Parse(hdr.Filename, data)
}

// GET /api/releases?state=&all=1&assigned=1  (translator+)
// Own releases by default. all=1 (verifier-class) lists everything;
// assigned=1 (verifier-class) is a verifier's working queue: releases routed
// to them plus unrouted ones.
func (h *Handler) listReleases(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	uploaderID := &middleware.UserFrom(r).UserID
	var assignedTo *int
	switch {
	case r.URL.Query().Get("assigned") == "1":
		if !canReview(r) {
			httpx.Error(w, http.StatusForbidden, "Insufficient permissions")
			return
		}
		uploaderID = nil
		assignedTo = &middleware.UserFrom(r).UserID
	case r.URL.Query().Get("all") == "1":
		if !canReview(r) {
			httpx.Error(w, http.StatusForbidden, "Insufficient permissions")
			return
		}
		uploaderID = nil
	}
	rels, err := h.repo.Releases(r.Context(), state, uploaderID, assignedTo)
	if err != nil {
		httpx.Internal(w, "list releases", err)
		return
	}
	for i := range rels {
		h.deriveRelease(&rels[i])
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": rels})
}

// GET /api/releases/{id}
func (h *Handler) getRelease(w http.ResponseWriter, r *http.Request) {
	if rel := h.loadRelease(w, r); rel != nil {
		httpx.JSON(w, http.StatusOK, map[string]any{"data": rel})
	}
}

// GET /api/releases/{id}/events — the editor grid.
func (h *Handler) releaseEvents(w http.ResponseWriter, r *http.Request) {
	rel := h.loadRelease(w, r)
	if rel == nil {
		return
	}
	events, err := h.repo.EventsByRelease(r.Context(), rel.ID)
	if err != nil {
		httpx.Internal(w, "fetch events", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": events})
}

// PUT /api/releases/{id}/events/{idx} — per-row autosave from the editor.
// The uploader edits drafts; a verifier may fix lines in review directly.
func (h *Handler) updateReleaseEvent(w http.ResponseWriter, r *http.Request) {
	rel := h.loadRelease(w, r)
	if rel == nil {
		return
	}
	editable := rel.State == "draft" || rel.State == "changes_requested" ||
		(rel.State == "in_review" && canReview(r))
	if !editable {
		httpx.Error(w, http.StatusConflict, "Release is not editable in state '"+rel.State+"'")
		return
	}
	idx, ok := httpx.IntParam(chi.URLParam(r, "idx"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid event index")
		return
	}
	var body struct {
		RoText *string `json:"roText"`
	}
	if err := httpx.Decode(r, &body); err != nil || body.RoText == nil {
		httpx.Error(w, http.StatusBadRequest, "roText is required")
		return
	}
	if err := h.repo.UpdateEventRO(r.Context(), rel.ID, idx, *body.RoText); err != nil {
		notFoundOr(w, err, "Event not found", "update event")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Saved"})
}

// GET /api/releases/{id}/video — streams the staging file with Range support
// (the native <video> preview scrubs through this).
func (h *Handler) releaseVideo(w http.ResponseWriter, r *http.Request) {
	rel := h.loadRelease(w, r)
	if rel == nil {
		return
	}
	// In the bucket: redirect rather than proxy. The preview then costs this
	// server no bandwidth at all, which it did when the file was local.
	if rel.R2Key != nil && *rel.R2Key != "" && h.storage != nil {
		url, err := h.storage.PresignGet(r.Context(), *rel.R2Key, presignTTL)
		if err != nil {
			httpx.Internal(w, "presign video", err)
			return
		}
		http.Redirect(w, r, url, http.StatusFound)
		return
	}
	if rel.Medium == "manga" || rel.StagingPath == nil {
		httpx.Error(w, http.StatusNotFound, "Staging video no longer exists")
		return
	}
	http.ServeFile(w, r, filepath.Join(h.cfg.StagingDir, filepath.FromSlash(*rel.StagingPath)))
}

// downloadBase builds a filename stem for a release download, e.g.
// "Sousou-no-Frieren-ep-01" — series title (or proposed) + episode/chapter.
func downloadBase(rel *model.Release) string {
	name := "release"
	switch {
	case rel.AnimeTitle != nil && *rel.AnimeTitle != "":
		name = *rel.AnimeTitle
	case rel.MangaTitle != nil && *rel.MangaTitle != "":
		name = *rel.MangaTitle
	case rel.ProposedTitle != nil && *rel.ProposedTitle != "":
		name = *rel.ProposedTitle
	}
	if rel.EpisodeNumber != nil {
		name += fmt.Sprintf(" ep %02d", *rel.EpisodeNumber)
	} else if rel.ChapterNumber != nil {
		name += fmt.Sprintf(" cap %g", *rel.ChapterNumber)
	}
	if slug := asciiSlug(name); slug != "" {
		return slug
	}
	return "release-" + strconv.Itoa(rel.ID)
}

// asciiSlug keeps a string safe for a Content-Disposition filename: letters,
// digits and hyphens only (spaces/underscores → hyphens, everything else dropped).
func asciiSlug(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ', r == '-', r == '_':
			b.WriteByte('-')
		}
	}
	return b.String()
}

// releaseCues turns a release's RO rows into subtitle cues (untranslated rows
// stay silent, same as the published .vtt).
func (h *Handler) releaseCues(ctx context.Context, releaseID int) ([]subs.Event, error) {
	events, err := h.repo.EventsByRelease(ctx, releaseID)
	if err != nil {
		return nil, err
	}
	cues := make([]subs.Event, 0, len(events))
	for _, e := range events {
		cues = append(cues, subs.Event{StartMs: e.StartMs, EndMs: e.EndMs, Text: e.RoText})
	}
	return cues, nil
}

// GET /api/releases/{id}/subtitle.srt — the RO track as SubRip, to upload to a
// host as a sidecar subtitle file.
func (h *Handler) releaseSubtitleSRT(w http.ResponseWriter, r *http.Request) {
	rel := h.loadRelease(w, r)
	if rel == nil {
		return
	}
	cues, err := h.releaseCues(r.Context(), rel.ID)
	if err != nil {
		httpx.Internal(w, "fetch events", err)
		return
	}
	w.Header().Set("Content-Type", "application/x-subrip; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.ro.srt"`, downloadBase(rel)))
	_, _ = io.WriteString(w, subs.WriteSRT(cues))
}

// GET /api/releases/{id}/download.mp4 — the staged video rewrapped as MP4, no
// subtitle and no re-encode. This is the file to upload to a host for an
// extract source: hosts that don't transcode hand back the same container they
// were given, and browsers won't play the Matroska that /download produces.
// Our own player gets the RO track from us, not from the file.
func (h *Handler) releaseDownloadMP4(w http.ResponseWriter, r *http.Request) {
	rel := h.loadRelease(w, r)
	if rel == nil {
		return
	}
	if rel.Medium == "manga" || rel.StagingPath == nil {
		httpx.Error(w, http.StatusNotFound, "Nu există video în staging pentru acest release")
		return
	}
	if !subs.HasFFmpeg() {
		httpx.Error(w, http.StatusServiceUnavailable,
			"Conversia în MP4 nu e disponibilă (lipsește ffmpeg)")
		return
	}
	// +faststart needs a seekable output, so this lands on disk first. Serving
	// the file (rather than a pipe) also makes the download resumable — these
	// are gigabyte-sized and translators are not always on good connections.
	tmp, err := os.CreateTemp(h.cfg.StagingDir, "ak-mp4-*.mp4")
	if err != nil {
		httpx.Internal(w, "temp mp4", err)
		return
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	videoPath := filepath.Join(h.cfg.StagingDir, filepath.FromSlash(*rel.StagingPath))
	if err := subs.RemuxMP4(r.Context(), videoPath, tmp.Name()); err != nil {
		httpx.Internal(w, "remux mp4", err)
		return
	}
	f, err := os.Open(tmp.Name())
	if err != nil {
		httpx.Internal(w, "open mp4", err)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		httpx.Internal(w, "stat mp4", err)
		return
	}
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.mp4"`, downloadBase(rel)))
	http.ServeContent(w, r, "", st.ModTime(), f)
}

// GET /api/releases/{id}/download — the staged video with the RO subtitle muxed
// in as a soft track (fast, no re-encode). One self-contained file to keep or to
// hand to someone; for uploading to a host, use download.mp4 instead.
func (h *Handler) releaseDownload(w http.ResponseWriter, r *http.Request) {
	rel := h.loadRelease(w, r)
	if rel == nil {
		return
	}
	if rel.Medium == "manga" || rel.StagingPath == nil {
		httpx.Error(w, http.StatusNotFound, "Nu există video în staging pentru acest release")
		return
	}
	if !subs.HasFFmpeg() {
		httpx.Error(w, http.StatusServiceUnavailable,
			"Muxarea subtitrării nu e disponibilă (lipsește ffmpeg) — descarcă separat video-ul și subtitrarea .srt")
		return
	}
	cues, err := h.releaseCues(r.Context(), rel.ID)
	if err != nil {
		httpx.Internal(w, "fetch events", err)
		return
	}
	tmp, err := os.CreateTemp("", "ak-ro-*.srt")
	if err != nil {
		httpx.Internal(w, "temp sub", err)
		return
	}
	defer os.Remove(tmp.Name())
	_, _ = tmp.WriteString(subs.WriteSRT(cues))
	tmp.Close()

	videoPath := filepath.Join(h.cfg.StagingDir, filepath.FromSlash(*rel.StagingPath))
	w.Header().Set("Content-Type", "video/x-matroska")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.ro.mkv"`, downloadBase(rel)))
	if err := subs.MuxSoftSub(r.Context(), videoPath, tmp.Name(), w); err != nil {
		// headers may already be flushed; log and let the truncated download fail
		slog.Error("mux soft sub", "release", rel.ID, "err", err)
	}
}

// GET /api/releases/{id}/draft.vtt — the RO draft as WebVTT, for the live
// <track> in the editor preview. Untranslated rows are simply absent.
func (h *Handler) releaseDraftVTT(w http.ResponseWriter, r *http.Request) {
	rel := h.loadRelease(w, r)
	if rel == nil {
		return
	}
	events, err := h.repo.EventsByRelease(r.Context(), rel.ID)
	if err != nil {
		httpx.Internal(w, "fetch events", err)
		return
	}
	cues := make([]subs.Event, len(events))
	for i, e := range events {
		cues[i] = subs.Event{Idx: e.Idx, StartMs: e.StartMs, EndMs: e.EndMs, Text: e.RoText}
	}
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store") // it changes on every keystroke
	_, _ = io.WriteString(w, subs.WriteVTT(cues))
}

// POST /api/releases/{id}/submit — draft → in_review (uploader only).
func (h *Handler) submitRelease(w http.ResponseWriter, r *http.Request) {
	rel := h.loadRelease(w, r)
	if rel == nil {
		return
	}
	if rel.UploaderID != middleware.UserFrom(r).UserID {
		httpx.Error(w, http.StatusForbidden, "Only the uploader can submit")
		return
	}
	if rel.Medium == "manga" { // review-ready = staged RO pages exist
		if len(h.stagingPages(rel.ID, "ro")) == 0 {
			httpx.Error(w, http.StatusBadRequest, "Nothing to review: no pages are staged")
			return
		}
		err := h.repo.SetReleaseState(r.Context(), rel.ID, []string{"draft", "changes_requested"}, "in_review", nil, nil)
		if err != nil {
			notFoundOr(w, err, "Release is not in a submittable state", "submit release")
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"message": "Trimis la verificare"})
		return
	}
	translated, total, err := h.repo.TranslatedEventCount(r.Context(), rel.ID)
	if err != nil {
		httpx.Internal(w, "check draft", err)
		return
	}
	if translated == 0 {
		httpx.Error(w, http.StatusBadRequest, "Nothing to review: no lines are translated yet")
		return
	}
	err = h.repo.SetReleaseState(r.Context(), rel.ID, []string{"draft", "changes_requested"}, "in_review", nil, nil)
	if err != nil {
		notFoundOr(w, err, "Release is not in a submittable state", "submit release")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"message":    "Trimis la verificare",
		"translated": translated,
		"total":      total,
	})
}

// POST /api/releases/{id}/approve  (verifier+) — in_review → approved.
// Approval is a quality verdict only; going live is the coordinator's publish
// step (below), where the series/episode mapping is confirmed and the source
// attached.
func (h *Handler) approveRelease(w http.ResponseWriter, r *http.Request) {
	rel := h.loadRelease(w, r)
	if rel == nil || !h.verdictAllowed(w, r, rel) {
		return
	}
	reviewer := middleware.UserFrom(r).UserID
	err := h.repo.SetReleaseState(r.Context(), rel.ID, []string{"in_review"}, "approved", &reviewer, nil)
	if err != nil {
		notFoundOr(w, err, "Release is not in review", "approve release")
		return
	}
	link := fmt.Sprintf("/translate/%d", rel.ID)
	h.notify(r.Context(), rel.UploaderID, "release",
		releaseLabel(rel)+" a fost aprobat — urmează publicarea.", &reviewer, &link)
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Aprobat — trimis la publicare"})
}

// POST /api/releases/{id}/publish  (coordinator/admin) — approved → published.
// Body (all optional unless the release has no series yet):
//
//	animeId        pin/override the series (required when the release only
//	               has a proposed title — import it from MAL first)
//	episodeNumber  override the translator's episode number
//	sources        [{hostingUrl, kind, provider, quality, language}] — the
//	               episode's sources, each validated like the admin form
//	               (`source` singular is accepted too)
//
// Publishing: episode find-or-create → RO .vtt under uploads → published
// subtitle upsert → optional source link → state published. The 4.5 host
// fan-out (video mirrors) will extend this once host accounts exist.
func (h *Handler) publishRelease(w http.ResponseWriter, r *http.Request) {
	if !canPublish(r) {
		httpx.Error(w, http.StatusForbidden, "Publishing is for coordinators and admins")
		return
	}
	rel := h.loadRelease(w, r)
	if rel == nil {
		return
	}
	if rel.State != "approved" {
		httpx.Error(w, http.StatusConflict, "Only approved releases can be published (current state: '"+rel.State+"')")
		return
	}
	if rel.Medium == "manga" {
		h.publishMangaRelease(w, r, rel)
		return
	}

	type pubSource struct {
		HostingURL  string  `json:"hostingUrl"`
		Kind        string  `json:"kind"`
		Provider    *string `json:"provider"`
		ProviderRef *string `json:"providerRef"`
		Quality     *string `json:"quality"`
		Language    string  `json:"language"`
	}
	var body struct {
		AnimeID       *int        `json:"animeId"`
		EpisodeNumber *int        `json:"episodeNumber"`
		Source        *pubSource  `json:"source"`
		Sources       []pubSource `json:"sources"`
	}
	if err := httpx.Decode(r, &body); err != nil && !errors.Is(err, io.EOF) {
		httpx.Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	sources := body.Sources
	if body.Source != nil {
		sources = append(sources, *body.Source)
	}

	animeID := rel.AnimeID
	if body.AnimeID != nil {
		animeID = body.AnimeID
	}
	if animeID == nil {
		title := ""
		if rel.ProposedTitle != nil {
			title = " („" + *rel.ProposedTitle + "”)"
		}
		httpx.Error(w, http.StatusBadRequest, "Release-ul nu are încă o serie în catalog"+title+" — importă titlul din MAL și alege-l la publicare")
		return
	}
	epNum := 0
	if rel.EpisodeNumber != nil {
		epNum = *rel.EpisodeNumber
	}
	if body.EpisodeNumber != nil {
		epNum = *body.EpisodeNumber
	}
	if epNum < 1 {
		httpx.Error(w, http.StatusBadRequest, "Invalid episodeNumber")
		return
	}
	// An anime episode with no source is unwatchable: publishing it would strand
	// the episode (nothing to play) and drop the release from the queue — the
	// exact trap that loses the download. Require the host link. Upload the
	// episode (the download buttons) to a host first, then paste the URL here.
	if len(sources) == 0 {
		httpx.Error(w, http.StatusBadRequest,
			"Adaugă cel puțin o sursă video: descarcă episodul (video + subtitrare), urcă-l la un host, apoi lipește linkul aici.")
		return
	}
	// validate every source before touching anything — a bad URL must not
	// leave a half-published release behind (same rules as the admin form)
	for i := range sources {
		s := &sources[i]
		if err := validateHostingURL(s.HostingURL, h.cfg.ContentHosts); err != nil {
			httpx.Error(w, http.StatusBadRequest, "Sursa: "+err.Error())
			return
		}
		if s.Kind == "" {
			s.Kind = "embed"
		}
		if s.Kind != "embed" && s.Kind != "extract" {
			httpx.Error(w, http.StatusBadRequest, "Sursa: kind must be 'embed' or 'extract'")
			return
		}
		if s.Kind == "embed" {
			s.HostingURL = fileHostEmbed(s.HostingURL)
		}
		if s.Kind == "extract" && (s.Provider == nil || *s.Provider == "" ||
			s.ProviderRef == nil || *s.ProviderRef == "") {
			httpx.Error(w, http.StatusBadRequest, "Sursa: extract sources need provider and providerRef")
			return
		}
		if s.Language == "" {
			s.Language = "ro"
		}
	}

	// pin the confirmed mapping on the release (clears any proposed title)
	if rel.AnimeID == nil || *animeID != *rel.AnimeID ||
		rel.EpisodeNumber == nil || epNum != *rel.EpisodeNumber {
		if err := h.repo.SetReleaseTarget(r.Context(), rel.ID, *animeID, epNum); err != nil {
			if repo.IsForeignKeyViolation(err) {
				httpx.Error(w, http.StatusNotFound, "Anime not found")
				return
			}
			httpx.Internal(w, "retarget release", err)
			return
		}
	}

	ep, err := h.repo.EpisodeByNumber(r.Context(), *animeID, epNum)
	if errors.Is(err, repo.ErrNotFound) {
		title := fmt.Sprintf("Episodul %d", epNum)
		ep, err = h.repo.CreateEpisode(r.Context(), *animeID, epNum, repo.EpisodeInput{Title: &title})
		if errors.Is(err, repo.ErrExists) { // lost a race — someone made it first
			ep, err = h.repo.EpisodeByNumber(r.Context(), *animeID, epNum)
		}
	}
	if err != nil {
		httpx.Internal(w, "resolve episode", err)
		return
	}

	if err := h.publishSubtitle(r.Context(), rel, ep.ID); err != nil {
		httpx.Internal(w, "publish subtitle", err)
		return
	}

	for _, s := range sources {
		in := repo.LinkInput{EpisodeID: &ep.ID, HostingURL: s.HostingURL,
			Kind: s.Kind, Provider: s.Provider, ProviderRef: s.ProviderRef,
			Quality: s.Quality, Language: s.Language}
		if _, err := h.repo.AddContentLink(r.Context(), in); err != nil {
			httpx.Internal(w, "add source", err)
			return
		}
	}

	// the coordinator publishing it — recorded so the episode page can credit
	// all three roles, not just translator and verifier
	publisher := middleware.UserFrom(r).UserID
	if err := h.repo.SetReleaseState(r.Context(), rel.ID, []string{"approved"}, "published", nil, nil, publisher); err != nil {
		notFoundOr(w, err, "Release is no longer approved", "mark published")
		return
	}
	link := fmt.Sprintf("/anime/%d/episode/%d", *animeID, epNum)
	h.notify(r.Context(), rel.UploaderID, "release",
		fmt.Sprintf("Traducerea ta pentru episodul %d e live — subtitrarea RO a fost publicată.", epNum),
		&publisher, &link)

	// Tell the members tracking this series. The watchlist IS the subscription
	// here: 'watching' or 'plan-to-watch' means "I want more of this", so there
	// is no separate bell to find and nothing to opt into.
	//
	// This fires ONLY here, in the coordinator's publish action. Importing a
	// catalog writes episodes and content_links directly and never reaches this
	// handler, so no import — however large — can notify anyone.
	h.notifyWatchers(r.Context(), *animeID, epNum, link, rel.UploaderID, publisher)
	httpx.JSON(w, http.StatusOK, map[string]any{
		"message":   "Publicat — subtitrarea RO e live pe episod",
		"episodeId": ep.ID,
		"animeId":   *animeID,
	})
}

// publishSubtitle serializes the release's RO rows to a .vtt under uploads
// and makes it the episode's live RO track.
func (h *Handler) publishSubtitle(ctx context.Context, rel *model.Release, episodeID int) error {
	events, err := h.repo.EventsByRelease(ctx, rel.ID)
	if err != nil {
		return fmt.Errorf("fetch events: %w", err)
	}
	if err := h.writeTrack(ctx, rel, episodeID, "ro", "Română", events,
		func(e model.SubtitleEvent) string { return e.RoText }); err != nil {
		return err
	}
	// The English source travels with every row already (subtitle_events.en_text),
	// so publishing it costs one more file and nothing else — and it is exactly
	// what you want to read against when a Romanian line looks wrong months
	// later. Releases that were uploaded as a finished RO sub have no EN text;
	// writeTrack skips those rather than publishing an empty track.
	return h.writeTrack(ctx, rel, episodeID, "en", "English", events,
		func(e model.SubtitleEvent) string { return e.EnText })
}

// writeTrack serializes one language out of the release's rows to a .vtt under
// uploads and makes it a live track on the episode. A track with no text at
// all is not written.
func (h *Handler) writeTrack(
	ctx context.Context, rel *model.Release, episodeID int,
	lang, label string, events []model.SubtitleEvent, text func(model.SubtitleEvent) string,
) error {
	cues := make([]subs.Event, 0, len(events))
	for _, e := range events {
		if strings.TrimSpace(text(e)) == "" {
			continue // untranslated rows stay silent, same as the draft preview
		}
		cues = append(cues, subs.Event{Idx: len(cues), StartMs: e.StartMs, EndMs: e.EndMs, Text: text(e)})
	}
	if len(cues) == 0 {
		return nil
	}

	dir := filepath.Join(h.cfg.UploadsDir, "subs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create subs dir: %w", err)
	}
	name := fmt.Sprintf("ep-%d-%s.vtt", episodeID, lang)
	if err := os.WriteFile(filepath.Join(dir, name), []byte(subs.WriteVTT(cues)), 0o644); err != nil {
		return fmt.Errorf("write %s vtt: %w", lang, err)
	}

	source := fmt.Sprintf("release:%d", rel.ID)
	if _, err := h.repo.UpsertPublishedSubtitle(ctx, repo.SubtitleInput{
		EpisodeID: episodeID, Language: lang, Label: &label, Format: "vtt",
		URL: "/uploads/subs/" + name, TranslatorID: &rel.UploaderID, SourceSub: &source,
	}); err != nil {
		return fmt.Errorf("upsert %s subtitle: %w", lang, err)
	}
	return nil
}

// POST /api/releases/{id}/request-changes  (verifier+) — back to the
// translator, notes required.
func (h *Handler) requestChanges(w http.ResponseWriter, r *http.Request) {
	rel := h.loadRelease(w, r)
	if rel == nil || !h.verdictAllowed(w, r, rel) {
		return
	}
	var body struct {
		Notes string `json:"notes"`
	}
	if err := httpx.Decode(r, &body); err != nil || strings.TrimSpace(body.Notes) == "" {
		httpx.Error(w, http.StatusBadRequest, "notes are required — tell the translator what to fix")
		return
	}
	reviewer := middleware.UserFrom(r).UserID
	err := h.repo.SetReleaseState(r.Context(), rel.ID, []string{"in_review"}, "changes_requested", &reviewer, &body.Notes)
	if err != nil {
		notFoundOr(w, err, "Release is not in review", "request changes")
		return
	}
	link := fmt.Sprintf("/translate/%d", rel.ID)
	h.notify(r.Context(), rel.UploaderID, "release",
		releaseLabel(rel)+" a primit observații de verificare.", &reviewer, &link)
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Trimis înapoi cu observații"})
}

// DELETE /api/releases/{id} — uploader abandons their own unfinished work;
// verifier-class can delete anything. Removes the staging files too.
func (h *Handler) deleteRelease(w http.ResponseWriter, r *http.Request) {
	rel := h.loadRelease(w, r)
	if rel == nil {
		return
	}
	own := rel.UploaderID == middleware.UserFrom(r).UserID
	unfinished := rel.State == "draft" || rel.State == "changes_requested"
	if !canReview(r) && !(own && unfinished) {
		httpx.Error(w, http.StatusForbidden, "Only unfinished releases can be deleted by their uploader")
		return
	}
	// Capture the bucket key before the row goes: after DeleteRelease there is
	// nothing left that knows the object exists, and it would sit in R2 until the
	// lifecycle rule swept it — or for ever, if the prefix rule is not set.
	var r2Key string
	if rel.R2Key != nil {
		r2Key = *rel.R2Key
	}
	if _, err := h.repo.DeleteRelease(r.Context(), rel.ID); err != nil {
		notFoundOr(w, err, "Release not found", "delete release")
		return
	}
	h.removeStaging(rel.ID)
	if r2Key != "" && h.storage != nil {
		if err := h.storage.Delete(r.Context(), r2Key); err != nil {
			slog.Warn("delete R2 video with release", "release", rel.ID, "key", r2Key, "err", err)
		}
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Release deleted"})
}

func (h *Handler) removeStaging(releaseID int) {
	if err := os.RemoveAll(filepath.Join(h.cfg.StagingDir, strconv.Itoa(releaseID))); err != nil {
		slog.Error("remove staging dir", "release", releaseID, "err", err)
	}
}

// staging lifetimes (architecture §3: staging is strictly transient).
const (
	// abandoned drafts: the whole release goes
	staleDraftDays = 30
	// published releases: the bytes go, the release row stays. The window is
	// the re-upload grace period — long enough to move an episode to another
	// host or fix a bad encode, short enough that disk trends to zero.
	publishedStagingDays = 30
)

// ExpireStaleStaging is the staging janitor, run by cmd/api twice a day. It
// does two different things:
//
//   - abandoned drafts (untouched for staleDraftDays) are deleted outright;
//   - published releases keep their row forever — credits and the hall of fame
//     read it — but lose the staged video after publishedStagingDays. Without
//     this half, staging grows without bound: every published episode used to
//     keep its master on disk permanently.
func (h *Handler) ExpireStaleStaging(ctx context.Context) {
	stale, err := h.repo.StaleReleases(ctx, time.Now().AddDate(0, 0, -staleDraftDays))
	if err != nil {
		slog.Error("list stale releases", "err", err)
	}
	for _, rel := range stale {
		if _, err := h.repo.DeleteRelease(ctx, rel.ID); err != nil {
			slog.Error("expire release", "id", rel.ID, "err", err)
			continue
		}
		h.removeStaging(rel.ID)
		slog.Info("expired stale release", "id", rel.ID, "medium", rel.Medium)
	}

	h.PurgePublishedVideo(ctx)
}

// PurgePublishedVideo drops the staged video of anything already published.
//
// Published means the episode is live and its subtitle track has been written to
// uploads/ — the track is the product and it is kept forever. The video is not
// ours to keep: it is a multi-gigabyte working copy of someone else's file, and
// storing it indefinitely is how a 75 GB disk fills up.
//
// The grace period exists for one case only: publishing the wrong thing, or the
// host upload failing, and needing the file back. Minutes, not days — long
// enough to notice and re-download, short enough that disk is bounded by
// throughput rather than by history. Runs on a short ticker, so the window is
// real rather than nominal.
//
// The hardsub artefact lives in the same directory and goes with it, which is
// why the row's hardsub state is cleared too: leaving it as 'done' would show a
// download button for a file that no longer exists.
func (h *Handler) PurgePublishedVideo(ctx context.Context) {
	done, err := h.repo.PublishedWithStaging(ctx, time.Now().Add(-h.cfg.PublishVideoGrace))
	if err != nil {
		slog.Error("list published releases with staging", "err", err)
		return
	}
	for _, rel := range done {
		// bytes first, pointer second. If we die in between, the row still
		// says staging_path IS NOT NULL, so the next run finds it again and
		// finishes the job (RemoveAll on a missing dir is a no-op). The other
		// order orphans the bytes permanently — the query would never match
		// that release again.
		h.removeStaging(rel.ID)
		if err := h.repo.ClearStagingPath(ctx, rel.ID); err != nil {
			slog.Error("clear staging path", "id", rel.ID, "err", err)
			continue
		}
		if rel.R2Key != nil && *rel.R2Key != "" && h.storage != nil {
			if err := h.storage.Delete(ctx, *rel.R2Key); err != nil {
				slog.Warn("delete published R2 video", "id", rel.ID, "err", err)
			} else if err := h.repo.SetReleaseR2Key(ctx, rel.ID, nil); err != nil {
				slog.Error("clear r2 key", "id", rel.ID, "err", err)
			}
		}
		if err := h.repo.ClearHardsub(ctx, rel.ID); err != nil {
			slog.Warn("clear hardsub after purge", "id", rel.ID, "err", err)
		}
		slog.Info("purged staging of published release", "id", rel.ID, "medium", rel.Medium)
	}
}
