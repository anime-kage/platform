package handler

// Chapter page endpoints for the native manga reader. The
// reader asks for a language and gets that edition plus the list of editions.
// Own pages (chapter_pages rows) always win; when a language exists only as
// an extract source (content_links kind='extract'), the pages are pulled from
// the third-party site at read time, cached briefly (extracted URLs expire),
// and served through the image proxy below — the browser never talks to the
// source site, and the proxy attaches whatever Referer it demands.

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"animekage/backend/internal/httpx"
	"animekage/backend/internal/model"
	"animekage/backend/internal/repo"
)

// pageSetTTL is shorter than MangaDex's ~15-minute URL validity so a cached
// set never outlives its URLs.
const pageSetTTL = 10 * time.Minute

type cachedPageSet struct {
	pages   []string
	referer string
	expires time.Time
}

type pageSetCache struct {
	mu sync.Mutex
	m  map[string]cachedPageSet
}

func (c *pageSetCache) get(key string) (cachedPageSet, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	set, ok := c.m[key]
	if !ok || time.Now().After(set.expires) {
		return cachedPageSet{}, false
	}
	return set, true
}

func (c *pageSetCache) put(key string, set cachedPageSet) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = make(map[string]cachedPageSet)
	}
	// drop stale entries so the map can't grow unbounded
	now := time.Now()
	for k, v := range c.m {
		if now.After(v.expires) {
			delete(c.m, k)
		}
	}
	c.m[key] = set
}

// extractedPages returns the extracted page set for one chapter+language,
// from cache or by running the extractor on the best healthy source.
func (h *Handler) extractedPages(ctx context.Context, chapterID int, lang string, links []model.ContentLink) (cachedPageSet, error) {
	key := fmt.Sprintf("%d:%s", chapterID, lang)
	if set, ok := h.pageCache.get(key); ok {
		return set, nil
	}
	for _, l := range links {
		if l.Language != lang || l.Provider == nil || l.ProviderRef == nil || !h.mangaext.Has(*l.Provider) {
			continue
		}
		res, err := h.mangaext.Extract(ctx, *l.Provider, *l.ProviderRef)
		if err != nil {
			continue // try the next source; the health checker will flag it
		}
		set := cachedPageSet{pages: res.Pages, referer: res.Referer, expires: time.Now().Add(pageSetTTL)}
		h.pageCache.put(key, set)
		return set, nil
	}
	return cachedPageSet{}, fmt.Errorf("no extractable source for chapter %d lang %s", chapterID, lang)
}

// proxyPagePath is what the reader receives instead of the raw extracted URL.
func proxyPagePath(chapterID, idx int, lang string) string {
	return fmt.Sprintf("/api/chapters/%d/pageimg/%d?lang=%s", chapterID, idx, url.QueryEscape(lang))
}

// GET /api/chapters/{id}/pages?lang=ro
func (h *Handler) chapterPages(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid chapter ID")
		return
	}
	exists, err := h.repo.ChapterExists(r.Context(), id)
	if err != nil {
		httpx.Internal(w, "fetch chapter pages", err)
		return
	}
	if !exists {
		httpx.Error(w, http.StatusNotFound, "Chapter not found")
		return
	}

	ownLangs, err := h.repo.ChapterPageLanguages(r.Context(), id)
	if err != nil {
		httpx.Internal(w, "fetch chapter pages", err)
		return
	}
	extLinks, err := h.repo.ChapterExtractSources(r.Context(), id)
	if err != nil {
		httpx.Internal(w, "fetch chapter pages", err)
		return
	}

	// available editions: own first (already RO-first), then extract-only
	// languages in source order
	own := make(map[string]bool, len(ownLangs))
	for _, l := range ownLangs {
		own[l] = true
	}
	langs := append([]string{}, ownLangs...)
	for _, l := range extLinks {
		if h.mangaext.Has(strval(l.Provider)) && !contains(langs, l.Language) {
			langs = append(langs, l.Language)
		}
	}

	// requested language if it exists, else RO, else the first edition
	lang := r.URL.Query().Get("lang")
	chosen := ""
	if len(langs) > 0 {
		chosen = langs[0]
		if contains(langs, "ro") {
			chosen = "ro"
		}
		for _, l := range langs {
			if l == lang {
				chosen = l
			}
		}
	}

	pages := []string{}
	if chosen != "" && own[chosen] {
		if pages, err = h.repo.ChapterPageSet(r.Context(), id, chosen); err != nil {
			httpx.Internal(w, "fetch chapter pages", err)
			return
		}
	} else if chosen != "" {
		set, err := h.extractedPages(r.Context(), id, chosen, extLinks)
		if err == nil {
			for i := range set.pages {
				pages = append(pages, proxyPagePath(id, i, chosen))
			}
		}
		// extraction failure degrades to an empty edition — the reader falls
		// back to the iframe, same as before 6.2
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"language": chosen, "languages": langs, "pages": pages,
	}})
}

// GET /api/chapters/{id}/pageimg/{idx}?lang=ro — the Referer-fixing image
// proxy. Serves only URLs that came out of an extractor for this
// exact chapter+language, so it can't be used as an open proxy.
func (h *Handler) chapterPageImage(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	idx, ok2 := httpx.IntParam(chi.URLParam(r, "idx"))
	if !ok || !ok2 || idx < 0 {
		httpx.Error(w, http.StatusBadRequest, "Invalid page reference")
		return
	}
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = "ro"
	}

	key := fmt.Sprintf("%d:%s", id, lang)
	set, cached := h.pageCache.get(key)
	if !cached {
		links, err := h.repo.ChapterExtractSources(r.Context(), id)
		if err != nil {
			httpx.Internal(w, "proxy chapter page", err)
			return
		}
		if set, err = h.extractedPages(r.Context(), id, lang, links); err != nil {
			httpx.Error(w, http.StatusNotFound, "Page not available")
			return
		}
	}
	if idx >= len(set.pages) {
		httpx.Error(w, http.StatusNotFound, "Page not available")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, set.pages[idx], nil)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "Source unreachable")
		return
	}
	req.Header.Set("User-Agent", "AnimeKage/1.0")
	if set.referer != "" {
		req.Header.Set("Referer", set.referer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "Source unreachable")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		httpx.Error(w, http.StatusBadGateway, "Source refused the page")
		return
	}

	if ct := resp.Header.Get("Content-Type"); strings.HasPrefix(ct, "image/") {
		w.Header().Set("Content-Type", ct)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		w.Header().Set("Content-Length", cl)
	}
	// the underlying URL rotates, but the page itself is immutable
	w.Header().Set("Cache-Control", "public, max-age=86400")
	io.Copy(w, resp.Body) //nolint:errcheck // client hangups are not our error
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func strval(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

type pagesBody struct {
	Language string   `json:"language"`
	URLs     []string `json:"urls"`
}

// PUT /api/chapters/{id}/pages  (admin/translator) — replace one edition
func (h *Handler) setChapterPages(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid chapter ID")
		return
	}
	var body pagesBody
	if err := httpx.Decode(r, &body); err != nil || len(body.URLs) == 0 {
		httpx.Error(w, http.StatusBadRequest, "urls are required")
		return
	}
	if body.Language == "" {
		body.Language = "ro"
	}
	if len(body.URLs) > 500 {
		httpx.Error(w, http.StatusBadRequest, "Too many pages (max 500)")
		return
	}
	for _, u := range body.URLs {
		if err := validateHostingURL(u, h.cfg.ContentHosts); err != nil {
			httpx.Error(w, http.StatusBadRequest, err.Error()+": "+u)
			return
		}
	}
	exists, err := h.repo.ChapterExists(r.Context(), id)
	if err != nil {
		httpx.Internal(w, "set chapter pages", err)
		return
	}
	if !exists {
		httpx.Error(w, http.StatusNotFound, "Chapter not found")
		return
	}
	if err := h.repo.ReplaceChapterPages(r.Context(), id, body.Language, body.URLs); err != nil {
		httpx.Internal(w, "set chapter pages", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"message": "Pages saved", "count": len(body.URLs)})
}

// storePageImage persists one page image and returns its public URL: R2 when
// configured, else local UPLOADS_DIR — either way the URL works in
// the reader (the ApiClient absolutizes /-relative URLs). Deterministic names
// mean re-uploading an edition overwrites in place instead of orphaning.
func (h *Handler) storePageImage(ctx context.Context, ref *repo.ChapterRef, chapterID int, lang string, idx int, contentType string, data []byte) (string, error) {
	ext := avatarExt[contentType]
	name := fmt.Sprintf("%03d.%s", idx+1, ext)
	if h.storage != nil {
		chap := strconv.FormatFloat(ref.ChapterNumber, 'f', -1, 64)
		key := fmt.Sprintf("manga/%d/%s/%s/%s", ref.MangaID, chap, lang, name)
		return h.storage.Put(ctx, key, contentType, bytes.NewReader(data))
	}
	dir := filepath.Join(h.cfg.UploadsDir, "manga", strconv.Itoa(chapterID), lang)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("/uploads/manga/%d/%s/%s", chapterID, lang, name), nil
}

// POST /api/chapters/{id}/pages/upload  (admin/translator) — multipart page
// images under "pages", ordered by filename (zero-pad to be safe). Replaces
// the whole edition, same as PUT /pages with URLs.
func (h *Handler) uploadChapterPages(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid chapter ID")
		return
	}
	const maxTotal = 300 << 20 // 300 MB per edition
	r.Body = http.MaxBytesReader(w, r.Body, maxTotal)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Upload prea mare (max 300MB per capitol)")
		return
	}
	lang := r.FormValue("lang")
	if lang == "" {
		lang = "ro"
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
	sort.Slice(files, func(i, j int) bool { return files[i].Filename < files[j].Filename })

	ref, err := h.repo.ChapterRef(r.Context(), id)
	if err != nil {
		notFoundOr(w, err, "Chapter not found", "upload chapter pages")
		return
	}

	urls := make([]string, 0, len(files))
	for i, fh := range files {
		f, err := fh.Open()
		if err != nil {
			httpx.Internal(w, "upload chapter pages", err)
			return
		}
		data, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			httpx.Internal(w, "upload chapter pages", err)
			return
		}
		ct := http.DetectContentType(data)
		if _, ok := avatarExt[ct]; !ok {
			httpx.Error(w, http.StatusBadRequest, "Doar imagini JPEG, PNG, WebP sau GIF: "+fh.Filename)
			return
		}
		url, err := h.storePageImage(r.Context(), ref, id, lang, i, ct, data)
		if err != nil {
			httpx.Internal(w, "upload chapter pages", err)
			return
		}
		urls = append(urls, url)
	}

	if err := h.repo.ReplaceChapterPages(r.Context(), id, lang, urls); err != nil {
		httpx.Internal(w, "upload chapter pages", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"message": "Pagini încărcate", "count": len(urls), "urls": urls,
		"storage": map[bool]string{true: "r2", false: "local"}[h.storage != nil],
	})
}

// DELETE /api/chapters/{id}/pages?lang=ro  (admin/translator)
func (h *Handler) deleteChapterPages(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IntParam(chi.URLParam(r, "id"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Invalid chapter ID")
		return
	}
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = "ro"
	}
	if err := h.repo.DeleteChapterPages(r.Context(), id, lang); err != nil {
		notFoundOr(w, err, "No pages for that language", "delete chapter pages")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Pages deleted"})
}
