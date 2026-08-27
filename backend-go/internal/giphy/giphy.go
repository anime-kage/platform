// Package giphy is a minimal Giphy client for the GIF picker.
//
// It lives server-side on purpose. Giphy's own advice is to use their SDK in the
// browser, but that ships the API key to every visitor, and the free tier allows
// only 100 API calls an hour — a quota you cannot protect, pool or cache if each
// browser spends it independently. Proxying gives us one shared cache, one place
// to pin the content rating, and a key that stays in .env with the others.
//
// Worth being clear about what the quota covers: search and trending. The GIF
// URLs handed back point at media*.giphy.com and carry no key, so *displaying*
// GIFs — a page with a hundred of them — costs nothing. Only looking for one does.
package giphy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	apiBase = "https://api.giphy.com/v1/gifs"

	// Excludes Giphy's "r" tier (suggestive / violence / profanity). Pinned
	// server-side so a client cannot ask for anything looser.
	rating = "pg-13"

	// No `lang`: it is a hint for interpreting the query, NOT a filter on the
	// results — "naruto" with lang=ro returns the same 500 GIFs in a slightly
	// different order. Giphy's tag data is overwhelmingly English, so leaving it
	// off gives better matches than pretending otherwise.

	trendingTTL = 15 * time.Minute // identical for everyone, barely changes
	searchTTL   = time.Hour        // people search the same handful of things
	maxLimit    = 24
)

// GIF is the slice of Giphy's response the picker needs.
type GIF struct {
	ID string `json:"id"`
	// Title is Giphy's own; used as alt text, so it matters for screen readers.
	Title string `json:"title"`
	// URL is the still-sized animated GIF the message embeds.
	URL string `json:"url"`
	// Preview is a smaller copy for the picker grid — loading full-size GIFs
	// into a 24-cell grid is megabytes of animation nobody asked for.
	Preview string `json:"preview"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
}

type Client struct {
	key  string
	http *http.Client

	mu    sync.Mutex
	cache map[string]entry
}

type entry struct {
	gifs []GIF
	exp  time.Time
}

func New(apiKey string) *Client {
	return &Client{
		key:   apiKey,
		http:  &http.Client{Timeout: 10 * time.Second},
		cache: map[string]entry{},
	}
}

// Enabled reports whether a key was configured. With none, the endpoints answer
// 503 and the picker hides itself rather than erroring on every keystroke.
func (c *Client) Enabled() bool { return c != nil && c.key != "" }

// Trending is the picker's default view.
func (c *Client) Trending(limit int) ([]GIF, error) {
	return c.fetch("trending", "trending", url.Values{}, limit, trendingTTL)
}

// Search looks up a query. The cache key is the normalised query, so "Naruto "
// and "naruto" are one entry.
func (c *Client) Search(q string, limit int) ([]GIF, error) {
	norm := strings.ToLower(strings.TrimSpace(q))
	return c.fetch("search", "q:"+norm, url.Values{"q": {norm}}, limit, searchTTL)
}

func (c *Client) fetch(endpoint, cacheKey string, params url.Values, limit int, ttl time.Duration) ([]GIF, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("giphy: no API key configured")
	}
	if limit <= 0 || limit > maxLimit {
		limit = maxLimit
	}
	// Cache on the key alone, not key+limit: everything asks for the same page
	// size, and splitting the cache by it would multiply quota use for nothing.
	ck := endpoint + "|" + cacheKey
	if g, ok := c.cached(ck); ok {
		return g, nil
	}

	params.Set("api_key", c.key)
	params.Set("limit", fmt.Sprint(maxLimit))
	params.Set("rating", rating)
	params.Set("bundle", "messaging_non_clips") // the sizes a chat embed wants

	resp, err := c.http.Get(apiBase + "/" + endpoint + "?" + params.Encode())
	if err != nil {
		return nil, fmt.Errorf("giphy request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, ErrRateLimited
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("giphy responded %d", resp.StatusCode)
	}

	var body struct {
		Data []struct {
			ID     string `json:"id"`
			Title  string `json:"title"`
			Images struct {
				FixedHeight      giphyImage `json:"fixed_height"`
				FixedHeightSmall giphyImage `json:"fixed_height_small"`
			} `json:"images"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("giphy decode: %w", err)
	}

	out := make([]GIF, 0, len(body.Data))
	for _, g := range body.Data {
		main := g.Images.FixedHeight
		if main.URL == "" {
			continue
		}
		prev := g.Images.FixedHeightSmall
		if prev.URL == "" {
			prev = main
		}
		out = append(out, GIF{
			ID: g.ID, Title: g.Title,
			URL: main.URL, Preview: prev.URL,
			Width: main.intW(), Height: main.intH(),
		})
	}
	c.store(ck, out, ttl)
	return out, nil
}

// ErrRateLimited is the free tier's 100 calls/hour running out. Surfaced
// distinctly so the UI can say "try again shortly" instead of "broken".
var ErrRateLimited = fmt.Errorf("giphy: rate limit reached")

// giphyImage — Giphy returns the numbers as strings.
type giphyImage struct {
	URL    string `json:"url"`
	Width  string `json:"width"`
	Height string `json:"height"`
}

func (i giphyImage) intW() int { return atoi(i.Width) }
func (i giphyImage) intH() int { return atoi(i.Height) }

func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func (c *Client) cached(k string) ([]GIF, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.cache[k]
	if !ok || time.Now().After(e.exp) {
		return nil, false
	}
	return e.gifs, true
}

func (c *Client) store(k string, g []GIF, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Cheap sweep so a long-lived process cannot grow the map without bound.
	if len(c.cache) > 500 {
		now := time.Now()
		for key, e := range c.cache {
			if now.After(e.exp) {
				delete(c.cache, key)
			}
		}
	}
	c.cache[k] = entry{gifs: g, exp: time.Now().Add(ttl)}
}
