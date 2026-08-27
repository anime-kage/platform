package resolver

// VTBE extractor.
//
// The one host among 91 Days' ten sources that lets our own player play the
// video. Tested August 2026 against episode 1's real uploads; the results are
// worth recording because they are the reason this file exists and the others
// do not:
//
//	vtbe       HLS, Access-Control-Allow-Origin: * on the master playlist, the
//	           child playlist AND the .ts segments, identical with our Referer,
//	           their Referer, or none → works in our player
//	sibnet     direct .mp4, but 206 with their Referer and 403 with ours →
//	           iframe-only
//	luluvdo    HLS with ACAO *, but the token 403s on almost every retry →
//	           not dependable enough to resolve at play time
//	voe        the only media URL in the page is a Big Buck Bunny sample — a
//	           decoy; the real source is behind their obfuscation
//	mp4upload,
//	vidara,
//	abyss      no media URL recoverable from the embed page
//	mega       end-to-end encrypted, the key lives in the URL fragment and
//	           decryption is client-side — an iframe is the only way by design
//	filemoon,
//	doodstream no CORS on playlist or segments (see filemoon.go)
//
// Unlike Filemoon, VTBE's playlist carries ACAO: *, so nothing here needs the
// stream proxy — the browser fetches the CDN directly, which is the $0/mo side
// of architecture §6.
//
// The path token in the master URL is minted per page load, so the URL must be
// resolved at play time and must never be cached in the DB. That is already how
// the stream endpoint works.

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

const vtbeHost = "vtbe.to"

var (
	// XFileSharing-style file code, e.g. "lfj8p3jt0xp4".
	vtbeCodeRe = regexp.MustCompile(`(?i)^[a-z0-9]{8,24}$`)
	// Accepts every page shape they hand out: /embed-CODE.html, /CODE,
	// /CODE.html, /e/CODE.
	vtbePathRe = regexp.MustCompile(`(?i)/(?:embed-|e/)?([a-z0-9]{8,24})(?:\.html)?/?$`)
	// The player config holds the master playlist. Their URL has a literal
	// comma-delimited token segment ending in ".urlset", which no generic
	// "https://…m3u8" pattern would survive intact, so match it explicitly and
	// fall back to a plain .m3u8 for when they change the shape.
	vtbeURLSetRe = regexp.MustCompile(`https?://[^\s"'\\<>]+?\.urlset/master\.m3u8`)
	vtbeM3U8Re   = regexp.MustCompile(`https?://[^\s"'\\<>]+?\.m3u8[^\s"'\\<>]*`)
)

type VTBE struct {
	// Client is optional — the zero value uses a sane default. Injected in
	// tests so no network is touched.
	Client *http.Client
}

func (VTBE) Name() string { return "vtbe" }

func (v VTBE) client() *http.Client {
	if v.Client != nil {
		return v.Client
	}
	return &http.Client{Timeout: 15 * time.Second}
}

// VTBECode pulls the file code out of whatever the uploader pasted — a bare
// code, or any of the page URLs VTBE hands out.
func VTBECode(ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if vtbeCodeRe.MatchString(ref) {
		return ref, true
	}
	u, err := url.Parse(ref)
	if err != nil || u.Host == "" {
		return "", false
	}
	m := vtbePathRe.FindStringSubmatch(u.Path)
	if m == nil {
		return "", false
	}
	// "embed" on its own is a path segment, not a code.
	if strings.EqualFold(m[1], "embed") {
		return "", false
	}
	return m[1], true
}

// vtbeEmbedURL is exported-ish for the test; kept unexported and small.
func vtbeEmbedURL(base, code string) string {
	return fmt.Sprintf("%s/embed-%s.html", strings.TrimRight(base, "/"), code)
}

func (v VTBE) Resolve(ctx context.Context, ref string) (*Result, error) {
	return v.resolveFrom(ctx, "https://"+vtbeHost, ref)
}

// resolveFrom is Resolve with an injectable base URL, so the test can point it
// at an httptest server.
func (v VTBE) resolveFrom(ctx context.Context, base, ref string) (*Result, error) {
	code, ok := VTBECode(ref)
	if !ok {
		return nil, fmt.Errorf("%w: not a VTBE file code or URL: %q", ErrUnresolvable, ref)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, vtbeEmbedURL(base, code), nil)
	if err != nil {
		return nil, err
	}
	// Their edge serves a stub to non-browser agents, same as Filemoon's.
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := v.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: fetch embed page: %v", ErrUnresolvable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: embed page status %d", ErrUnresolvable, resp.StatusCode)
	}
	// The real page is ~6 KB; the cap stops an unexpected redirect to something
	// huge from being used to exhaust memory.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("%w: read embed page: %v", ErrUnresolvable, err)
	}
	// Their JS escapes the slashes.
	page := strings.ReplaceAll(string(body), `\/`, "/")

	manifest := ""
	if m := vtbeURLSetRe.FindString(page); m != "" {
		manifest = m
	} else if m := vtbeM3U8Re.FindString(page); m != "" {
		manifest = m
	}
	if manifest == "" {
		// A deleted file still renders a page, just without a playlist — which
		// is exactly the signal that the file is gone.
		return nil, fmt.Errorf("%w: no playlist on VTBE page for %s (file deleted?)", ErrUnresolvable, code)
	}

	return &Result{
		ManifestURL: manifest,
		Kind:        "hls",
		// No Referer header: their CDN answers with ACAO * and ignores Referer
		// entirely (verified with theirs, ours, and none). Sending one would
		// imply a constraint that does not exist, and Probe would then be
		// testing something the browser will not replicate.
	}, nil
}
