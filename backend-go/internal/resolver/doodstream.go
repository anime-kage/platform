package resolver

// DoodStream extractor. DoodStream runs
// under many domains (playmogo.com, dood.*, ds2play, …), all serving the same
// player, so the ref carries the host as well as the file code.
//
// Their embed page holds a "/pass_md5/…" path. Fetching it with the embed page
// as Referer returns a URL *prefix*; the player appends ten random characters
// and the token to make the final media URL. That URL is the uploaded file
// itself — DoodStream does not transcode — served with
// `Access-Control-Allow-Origin: *` and honouring Range, which is what makes our
// own player (and therefore our own subtitles and skip marks) possible here.
// Filemoon, by contrast, sends no CORS header at all:
//
// Verified against a live upload July 2026.

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	doodPassRe = regexp.MustCompile(`/pass_md5/[^\s"'<>]+`)
	doodCodeRe = regexp.MustCompile(`(?i)^/(?:e|d|f|v|embed|download)/([a-z0-9]{8,24})/?$`)
)

const doodRandLen = 10

type DoodStream struct {
	// Client is optional — the zero value uses a sane default. Injected in
	// tests so no network is touched.
	Client *http.Client
}

func (DoodStream) Name() string { return "doodstream" }

func (d DoodStream) client() *http.Client {
	if d.Client != nil {
		return d.Client
	}
	// never follow their redirects: without a Referer their edge bounces you
	// around a hotlink-protection chain, and each hop mints a fresh token
	return &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// doodRef splits the stored ref into the host and the file code. The ref is a
// full URL because the code alone is ambiguous across their domains.
func doodRef(ref string) (host, code string, ok bool) {
	ref = strings.TrimSpace(ref)
	if !strings.Contains(ref, "://") {
		ref = "https://" + ref
	}
	u, err := url.Parse(ref)
	if err != nil || u.Hostname() == "" {
		return "", "", false
	}
	m := doodCodeRe.FindStringSubmatch(u.Path)
	if m == nil {
		return "", "", false
	}
	return strings.ToLower(u.Hostname()), m[1], true
}

func (d DoodStream) Resolve(ctx context.Context, ref string) (*Result, error) {
	host, code, ok := doodRef(ref)
	if !ok {
		return nil, fmt.Errorf("%w: not a DoodStream embed URL: %q", ErrUnresolvable, ref)
	}
	embed := fmt.Sprintf("https://%s/e/%s", host, code)

	page, err := d.get(ctx, embed, "https://"+host+"/")
	if err != nil {
		return nil, fmt.Errorf("%w: fetch embed page: %v", ErrUnresolvable, err)
	}
	// a gone file still renders a page, titled "File not found"
	if strings.Contains(page, "File not found") || !strings.Contains(page, "video_player") {
		return nil, fmt.Errorf("%w: file %s is gone from %s", ErrUnresolvable, code, host)
	}
	pass := doodPassRe.FindString(page)
	if pass == "" {
		return nil, fmt.Errorf("%w: no pass_md5 path on the embed page for %s "+
			"(their player changed — the extractor needs updating)", ErrUnresolvable, code)
	}
	// the token is the last segment of the pass_md5 path
	token := pass[strings.LastIndexByte(pass, '/')+1:]

	// this call *must* carry the embed page as Referer, or it hands back a
	// prefix that only redirects
	prefix, err := d.get(ctx, "https://"+host+pass, embed)
	if err != nil {
		return nil, fmt.Errorf("%w: pass_md5: %v", ErrUnresolvable, err)
	}
	prefix = strings.TrimSpace(prefix)
	if !strings.HasPrefix(prefix, "https://") {
		return nil, fmt.Errorf("%w: pass_md5 returned no URL for %s", ErrUnresolvable, code)
	}

	media := fmt.Sprintf("%s%s?token=%s&expiry=%d",
		prefix, randAlnum(doodRandLen), token, time.Now().UnixMilli())

	return &Result{
		ManifestURL: media,
		// the original upload, untranscoded — a plain progressive file, so the
		// player needs no HLS.js. Upload MP4: their CDN serves whatever
		// container you gave it, and browsers will not play Matroska.
		Kind: "mp4",
		// informational: their CDN accepts any Referer, it just needs one, and
		// the browser always sends ours
		Headers: map[string]string{"Referer": embed},
	}, nil
}

func (d DoodStream) get(ctx context.Context, target, referer string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Referer", referer)

	resp, err := d.client().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	return string(body), err
}

const alnum = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// randAlnum is the tail their player appends to the prefix. Any characters
// work; they just have to be there and be the right length.
func randAlnum(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// the value is not a secret — a fixed tail still plays
		return strings.Repeat("A", n)
	}
	for i := range b {
		b[i] = alnum[int(b[i])%len(alnum)]
	}
	return string(b)
}
