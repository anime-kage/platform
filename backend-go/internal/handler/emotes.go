package handler

// Custom chat emotes — upload, list, disable, delete.
//
// How these end up looking uniform despite arriving at every size:
//
//  1. The chat renders every emote at a FIXED HEIGHT with width:auto. That is
//     what Twitch and 7TV do, and it is what actually does the work — a 300px
//     and a 40px upload land on screen the same height, aspect preserved.
//  2. Uploads are bounded here (dimensions, file size, aspect ratio), so a
//     wildly wrong image is refused at the door with a reason, rather than
//     silently rendering as a smear two words wide.
//  3. The admin form previews the emote at real chat size before saving, which
//     is the only way to be sure — a rule can reject the obviously broken, but
//     "does this read at 28px" is a judgement call.
//
// Deliberately NOT re-encoded server-side: most good emotes are animated GIFs,
// and resizing those frame-by-frame in Go is a large amount of machinery for a
// job the CSS above already does correctly.

import (
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"

	"animekage/backend/internal/httpx"
	"animekage/backend/internal/middleware"
	"animekage/backend/internal/repo"
)

const (
	emoteMaxUpload = 1 << 20 // 1 MB — an emote is a 112px picture, not a photo
	// The chat renders at 28px, so anything past this is only bandwidth — but
	// 512 rejected the 800px and 1024px files emote sites actually hand out,
	// which meant resizing every one by hand before uploading. The 1 MB cap
	// above is the real bandwidth guard; this now only stops the absurd.
	emoteMaxDim = 1024
	emoteMinDim = 16
	// Twitch's own emotes sit near 1:1 and cap around 3:1. Past that the image
	// renders as a sliver at a fixed height and reads as nothing.
	emoteMaxAspect = 3.0
)

// emoteCodeRe: what people type in chat. Letters and digits only — the
// tokenizer splits on whitespace, so a code with punctuation could never match.
var emoteCodeRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]{1,23}$`)

// GET /api/emotes — every member (the chat and the picker need it).
func (h *Handler) listEmotes(w http.ResponseWriter, r *http.Request) {
	// Admins asking from the management tab want the disabled ones too.
	all := r.URL.Query().Get("all") == "1" && canManageEmotes(r)
	rows, err := h.repo.Emotes(r.Context(), !all)
	if err != nil {
		httpx.Internal(w, "list emotes", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": rows})
}

// POST /api/emotes — multipart: `code` + `image`. Admin/coordinator.
func (h *Handler) createEmote(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, emoteMaxUpload+4096)
	if err := r.ParseMultipartForm(emoteMaxUpload); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.Error(w, http.StatusRequestEntityTooLarge, "Emote-ul trebuie să aibă sub 1 MB")
			return
		}
		httpx.Error(w, http.StatusBadRequest, "Formularul nu a putut fi citit")
		return
	}

	code := strings.TrimSpace(r.FormValue("code"))
	if !emoteCodeRe.MatchString(code) {
		httpx.Error(w, http.StatusBadRequest,
			"Numele trebuie să înceapă cu o literă și să aibă 2–24 caractere, doar litere și cifre")
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "Lipsește imaginea")
		return
	}
	defer file.Close()
	if header.Size > emoteMaxUpload {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "Emote-ul trebuie să aibă sub 1 MB")
		return
	}

	// DecodeConfig reads only the header, so this costs nothing even for a big
	// animated GIF — and it is what lets the rules below be about the picture
	// rather than about the file.
	cfg, format, err := image.DecodeConfig(file)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "Doar imagini PNG, GIF sau JPEG")
		return
	}
	if cfg.Width < emoteMinDim || cfg.Height < emoteMinDim {
		httpx.Error(w, http.StatusBadRequest, "Imaginea e prea mică (minim 16×16)")
		return
	}
	if cfg.Width > emoteMaxDim || cfg.Height > emoteMaxDim {
		httpx.Error(w, http.StatusBadRequest,
			"Imaginea e prea mare (maxim 1024×1024). Ideal ~112px înălțime.")
		return
	}
	if aspect := ratio(cfg.Width, cfg.Height); aspect > emoteMaxAspect {
		httpx.Error(w, http.StatusBadRequest,
			"Imaginea e prea alungită — în chat ar apărea ca o dungă. Folosește ceva apropiat de pătrat.")
		return
	}

	if _, err := file.Seek(0, 0); err != nil {
		httpx.Internal(w, "emote upload", err)
		return
	}
	url, err := h.storeUpload(file, "emotes", format)
	if err != nil {
		httpx.Internal(w, "emote upload", err)
		return
	}

	e, err := h.repo.CreateEmote(r.Context(), code, url, cfg.Width, cfg.Height,
		middleware.UserFrom(r).UserID)
	if errors.Is(err, repo.ErrExists) {
		httpx.Error(w, http.StatusConflict, "Există deja un emote cu numele ăsta")
		return
	}
	if err != nil {
		httpx.Internal(w, "create emote", err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"data": e})
}

// PATCH /api/emotes/{id} — {isActive}. Admin/coordinator.
func (h *Handler) updateEmote(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid emote ID")
		return
	}
	var body struct {
		IsActive *bool `json:"isActive"`
	}
	if err := httpx.Decode(r, &body); err != nil || body.IsActive == nil {
		httpx.Error(w, http.StatusBadRequest, "isActive lipsește")
		return
	}
	e, err := h.repo.SetEmoteActive(r.Context(), id, *body.IsActive)
	if err != nil {
		notFoundOr(w, err, "Emote-ul nu există", "update emote")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": e})
}

// DELETE /api/emotes/{id} — admin/coordinator.
func (h *Handler) deleteEmote(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid emote ID")
		return
	}
	if err := h.repo.DeleteEmote(r.Context(), id); err != nil {
		notFoundOr(w, err, "Emote-ul nu există", "delete emote")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Emote șters"})
}

func ratio(w, h int) float64 {
	if w == 0 || h == 0 {
		return 99
	}
	if w > h {
		return float64(w) / float64(h)
	}
	return float64(h) / float64(w)
}

func canManageEmotes(r *http.Request) bool {
	u := middleware.UserFrom(r)
	return u != nil && (u.Role == "admin" || u.Role == "coordinator")
}

// storeUpload writes an already-validated image and returns its public path.
func (h *Handler) storeUpload(f interface{ Read([]byte) (int, error) }, subdir, format string) (string, error) {
	dir := filepath.Join(h.cfg.UploadsDir, subdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := newUploadName(format)
	dst, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return "", err
	}
	defer dst.Close()
	buf := make([]byte, 32*1024)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return "", werr
			}
		}
		if err != nil {
			break
		}
	}
	return "/uploads/" + subdir + "/" + name, nil
}
