// Package mangaext turns a chapter source's (provider, providerRef) into an
// ordered list of page-image URLs. It is
// the manga twin of internal/resolver: one Extractor per third-party site,
// registry keyed by the provider name stored on content_links.
//
// Extracted URLs are often short-lived and/or hotlink-protected, so the API
// never hands them to the browser — the chapter image proxy (handler/pages.go)
// re-serves them with the Referer the site expects.
package mangaext

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"
)

// Result is one extracted edition of a chapter.
type Result struct {
	// Pages are absolute image URLs in reading order.
	Pages []string
	// Referer is the header value the image host requires; empty if none.
	Referer string
}

type Extractor interface {
	Name() string
	Extract(ctx context.Context, ref string) (*Result, error)
}

// ErrUnextractable means the ref is dead or the site changed — callers fall
// back to the next source (or the iframe reader).
var ErrUnextractable = errors.New("chapter pages could not be extracted")

type Registry struct {
	extractors map[string]Extractor
}

func NewRegistry(extractors ...Extractor) *Registry {
	r := &Registry{extractors: make(map[string]Extractor, len(extractors))}
	for _, e := range extractors {
		r.extractors[e.Name()] = e
	}
	return r
}

// Has reports whether a provider name belongs to a manga extractor — the
// admin test-source endpoint uses it to route between resolver and mangaext.
func (r *Registry) Has(provider string) bool {
	_, ok := r.extractors[provider]
	return ok
}

func (r *Registry) Extract(ctx context.Context, provider, ref string) (*Result, error) {
	e, ok := r.extractors[provider]
	if !ok {
		return nil, fmt.Errorf("no extractor registered for %q", provider)
	}
	return e.Extract(ctx, ref)
}

// Default returns the registry with every built-in extractor. baseURL
// overrides MangaDex's API root (tests stub it); empty means production.
func Default(mangadexBaseURL string) *Registry {
	return NewRegistry(NewMangaDex(mangadexBaseURL))
}

// ── MangaDex ──────────────────────────────────────────────────────────────────

// userAgent identifies us to source sites, per MangaDex API rules (anonymous
// clients without a UA are blocked).
const userAgent = "AnimeKage/1.0 (+https://anime-kage.example)"

// MangaDex extracts pages via the official at-home API: the providerRef is the
// MangaDex chapter UUID. Returned URLs are valid ~15 minutes, which is why the
// handler caches the set briefly and proxies the images.
type MangaDex struct {
	baseURL string
	client  *http.Client
}

func NewMangaDex(baseURL string) *MangaDex {
	if baseURL == "" {
		baseURL = "https://api.mangadex.org"
	}
	return &MangaDex{baseURL: baseURL, client: &http.Client{Timeout: 15 * time.Second}}
}

func (*MangaDex) Name() string { return "mangadex" }

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type atHomeResponse struct {
	Result  string `json:"result"`
	BaseURL string `json:"baseUrl"`
	Chapter struct {
		Hash string   `json:"hash"`
		Data []string `json:"data"`
	} `json:"chapter"`
}

func (m *MangaDex) Extract(ctx context.Context, ref string) (*Result, error) {
	if !uuidRe.MatchString(ref) {
		return nil, fmt.Errorf("%w: mangadex ref must be a chapter UUID", ErrUnextractable)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.baseURL+"/at-home/server/"+ref, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnextractable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: mangadex returned %d", ErrUnextractable, resp.StatusCode)
	}
	var body atHomeResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("%w: bad mangadex response: %v", ErrUnextractable, err)
	}
	if body.Result != "ok" || body.BaseURL == "" || body.Chapter.Hash == "" || len(body.Chapter.Data) == 0 {
		return nil, fmt.Errorf("%w: mangadex chapter has no pages", ErrUnextractable)
	}
	pages := make([]string, 0, len(body.Chapter.Data))
	for _, f := range body.Chapter.Data {
		pages = append(pages, body.BaseURL+"/data/"+body.Chapter.Hash+"/"+f)
	}
	return &Result{Pages: pages}, nil
}
