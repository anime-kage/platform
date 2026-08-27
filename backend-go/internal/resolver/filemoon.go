package resolver

// Filemoon extractor. Their embed page
// carries the HLS playlist URL in a data attribute; the playlist itself is
// generated per request and lists absolute, pre-signed segment URLs on their
// R2 bucket, so nothing here goes stale in the DB — we resolve at play time.
//
// The ref stored on content_links is the file code (e.g. "vgKGxRKV38Qn"). A
// full Filemoon URL is accepted too, so pasting what the dashboard shows works.
//
// Note their playlist and segments carry no Access-Control-Allow-Origin, so a
// browser cannot fetch either one directly from our origin. Playing this in our
// own player needs the stream proxy.4.

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// filemoonHost is the site that serves both the embed page and the playlist.
// Their other domains answer 200 with a single-page-app shell for any path,
// which makes them useless to us: a dead file is indistinguishable from a live
// one. Keep the resolver pointed at the host that answers with real HTML.
const filemoonHost = "filemoon.org"

// browserUA — their edge serves a different (useless) page to non-browser
// agents, and the same UA must be replayed when fetching the playlist.
const browserUA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 " +
	"(KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

var (
	filemoonCodeRe = regexp.MustCompile(`(?i)^[a-z0-9]{8,24}$`)
	filemoonPathRe = regexp.MustCompile(`(?i)/([a-z0-9]{8,24})(?:/(?:embed|watch|file|download|stream))?/?$`)
	filemoonHLSRe  = regexp.MustCompile(`data-hls-url="([^"]+)"`)
)

type Filemoon struct {
	// Client is optional — the zero value uses a sane default. Injected in
	// tests so no network is touched.
	Client *http.Client
}

func (Filemoon) Name() string { return "filemoon" }

func (f Filemoon) client() *http.Client {
	if f.Client != nil {
		return f.Client
	}
	return &http.Client{Timeout: 15 * time.Second}
}

// FilemoonCode pulls the file code out of whatever the uploader pasted — a
// bare code, or any of the page URLs Filemoon hands out.
func FilemoonCode(ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if filemoonCodeRe.MatchString(ref) {
		return ref, true
	}
	u, err := url.Parse(ref)
	if err != nil || u.Host == "" {
		return "", false
	}
	m := filemoonPathRe.FindStringSubmatch(u.Path)
	if m == nil {
		return "", false
	}
	return m[1], true
}

func (f Filemoon) Resolve(ctx context.Context, ref string) (*Result, error) {
	code, ok := FilemoonCode(ref)
	if !ok {
		return nil, fmt.Errorf("%w: not a Filemoon file code or URL: %q", ErrUnresolvable, ref)
	}
	embed := fmt.Sprintf("https://%s/en/%s/embed", filemoonHost, code)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, embed, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := f.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: fetch embed page: %v", ErrUnresolvable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: embed page status %d", ErrUnresolvable, resp.StatusCode)
	}
	// the page is ~270 KB; cap the read so an unexpected redirect to something
	// huge can't be used to exhaust memory
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("%w: read embed page: %v", ErrUnresolvable, err)
	}
	page := string(body)

	// A deleted file still renders a page — the player box is what proves the
	// file is actually there.
	if !strings.Contains(page, "filemoon-player-box-"+code) {
		return nil, fmt.Errorf("%w: file %s is gone from Filemoon", ErrUnresolvable, code)
	}

	m := filemoonHLSRe.FindStringSubmatch(page)
	if m == nil {
		return nil, fmt.Errorf("%w: no playlist URL on the embed page for %s "+
			"(their player changed — the extractor needs updating)", ErrUnresolvable, code)
	}

	return &Result{
		ManifestURL: html.UnescapeString(m[1]),
		Kind:        "hls",
		// their edge checks Referer on the playlist
		Headers: map[string]string{
			"Referer":    embed,
			"User-Agent": browserUA,
		},
	}, nil
}
