package resolver

// Luluvdo extractor.
//
// Added as a FALLBACK BEHIND VTBE, not as a peer, and the reason is worth
// stating: in testing (August 2026, 91 Days ep 1) its playlist carried
// Access-Control-Allow-Origin: * and played fine on one attempt, then answered
// 403 to a freshly minted token on the next two. Something in their token is
// bound to state this resolver does not reproduce — plausibly the IP that loaded
// the embed page, which would make it unusable from a browser regardless, since
// the page is loaded by our server and played by the viewer.
//
// It is registered anyway because `ExtractSources` orders by priority and the
// stream endpoint falls through to the next source when a resolve fails: with
// vtbe ranked above it, a dead Luluvdo costs one failed request and nothing
// else. If it turns out to work in practice, it is a second source for free; if
// it does not, it never gets reached.
//
// Their playlist URL is in the page either as plain text or inside a packed
// block, apparently at random, so both are searched (see unpack.go).

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const luluvdoHost = "luluvdo.com"

var (
	luluvdoCodeRe = regexp.MustCompile(`(?i)^[a-z0-9]{8,24}$`)
	luluvdoPathRe = regexp.MustCompile(`(?i)/(?:e/|embed-|d/)?([a-z0-9]{8,24})(?:\.html)?/?$`)
	luluvdoM3U8Re = regexp.MustCompile(`https?://[^\s"'\\<>]+?\.m3u8[^\s"'\\<>]*`)
)

type Luluvdo struct {
	Client *http.Client
}

func (Luluvdo) Name() string { return "luluvdo" }

func (l Luluvdo) client() *http.Client {
	if l.Client != nil {
		return l.Client
	}
	return &http.Client{Timeout: 15 * time.Second}
}

// LuluvdoCode pulls the file code out of a bare code or any of their page URLs.
func LuluvdoCode(ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if luluvdoCodeRe.MatchString(ref) {
		return ref, true
	}
	u, err := url.Parse(ref)
	if err != nil || u.Host == "" {
		return "", false
	}
	m := luluvdoPathRe.FindStringSubmatch(u.Path)
	if m == nil {
		return "", false
	}
	if strings.EqualFold(m[1], "embed") {
		return "", false
	}
	return m[1], true
}

func (l Luluvdo) Resolve(ctx context.Context, ref string) (*Result, error) {
	return l.resolveFrom(ctx, "https://"+luluvdoHost, ref)
}

func (l Luluvdo) resolveFrom(ctx context.Context, base, ref string) (*Result, error) {
	code, ok := LuluvdoCode(ref)
	if !ok {
		return nil, fmt.Errorf("%w: not a Luluvdo file code or URL: %q", ErrUnresolvable, ref)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/e/%s", strings.TrimRight(base, "/"), code), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := l.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: fetch embed page: %v", ErrUnresolvable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: embed page status %d", ErrUnresolvable, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("%w: read embed page: %v", ErrUnresolvable, err)
	}

	page := strings.ReplaceAll(string(body), `\/`, "/")
	manifest := luluvdoM3U8Re.FindString(page)
	if manifest == "" {
		// Not in the raw HTML on this load — try the packed block.
		manifest = luluvdoM3U8Re.FindString(strings.ReplaceAll(Unpack(page), `\/`, "/"))
	}
	if manifest == "" {
		return nil, fmt.Errorf("%w: no playlist on Luluvdo page for %s", ErrUnresolvable, code)
	}

	return &Result{ManifestURL: manifest, Kind: "hls"}, nil
}
