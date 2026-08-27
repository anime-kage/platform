// Package resolver turns a source's (provider, providerRef) into something the
// player can actually play — an HLS manifest or MP4 URL (
//). One Provider implementation per third-party host; the registry is
// keyed by the provider name stored on content_links.
//
// It lives in-process with the product API for now: the interface is the seam,
// and extracting it into a standalone service later is a deploy decision, not a
// rewrite.
package resolver

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Result is what a provider hands the player.
type Result struct {
	// ManifestURL points at the provider's CDN — segments must NOT be proxied
	// through us (architecture §6: that's the $0/mo vs $150/mo line).
	ManifestURL string `json:"manifestUrl"`
	// Kind is "hls" or "mp4" — tells the player whether it needs hls.js.
	Kind string `json:"kind"`
	// Headers the player (or a future proxy) must send, e.g. Referer.
	Headers map[string]string `json:"headers,omitempty"`
	// Subtitles the provider ships (ours come from the subtitles table instead).
	Subtitles []Subtitle `json:"subtitles,omitempty"`
}

type Subtitle struct {
	URL      string `json:"url"`
	Language string `json:"language"`
	Label    string `json:"label,omitempty"`
}

type Provider interface {
	Name() string
	Resolve(ctx context.Context, ref string) (*Result, error)
}

// ErrUnresolvable means the ref is dead or the provider changed — the health
// checker flips last_ok on it; the stream endpoint tries the next source.
var ErrUnresolvable = errors.New("source could not be resolved")

type Registry struct {
	providers map[string]Provider
}

func NewRegistry(providers ...Provider) *Registry {
	r := &Registry{providers: make(map[string]Provider, len(providers))}
	for _, p := range providers {
		r.providers[p.Name()] = p
	}
	return r
}

// Resolve dispatches to the named provider.
func (r *Registry) Resolve(ctx context.Context, provider, ref string) (*Result, error) {
	p, ok := r.providers[provider]
	if !ok {
		return nil, fmt.Errorf("no provider registered for %q", provider)
	}
	return p.Resolve(ctx, ref)
}

// Default returns the registry with every built-in provider.
func Default() *Registry {
	return NewRegistry(Direct{}, Filemoon{}, DoodStream{}, VTBE{}, Luluvdo{})
}

// ── direct ────────────────────────────────────────────────────────────────────

// Direct is the trivial provider: the ref IS the playable URL. Used for our own
// uploads where the host gives us a stable manifest, and for wiring/tests. It
// validates, it does not fetch — liveness is the health checker's job.
type Direct struct{}

func (Direct) Name() string { return "direct" }

func (Direct) Resolve(_ context.Context, ref string) (*Result, error) {
	u, err := url.Parse(ref)
	if err != nil || !u.IsAbs() || u.Scheme != "https" || u.Hostname() == "" {
		return nil, fmt.Errorf("%w: direct ref must be an absolute https URL", ErrUnresolvable)
	}
	// The ref must be the media itself. A file host's watch/embed page is the
	// common mistake: the player would feed an HTML document to <video> and
	// die on CORS, several layers away from the actual cause. Say it here.
	path := strings.ToLower(u.Path)
	kind := ""
	switch {
	case strings.HasSuffix(path, ".m3u8"):
		kind = "hls"
	case strings.HasSuffix(path, ".mp4"), strings.HasSuffix(path, ".webm"):
		kind = "mp4"
	default:
		return nil, fmt.Errorf(
			"%w: direct ref must point at the media file itself (.m3u8 or .mp4), not at a host's page — "+
				"a watch/embed page needs an extractor provider, or kind='embed'", ErrUnresolvable)
	}
	return &Result{ManifestURL: ref, Kind: kind}, nil
}
