package resolver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVTBECode(t *testing.T) {
	tests := []struct {
		ref  string
		want string
	}{
		{"lfj8p3jt0xp4", "lfj8p3jt0xp4"},
		{"https://vtbe.to/embed-lfj8p3jt0xp4.html", "lfj8p3jt0xp4"},
		{"https://vtbe.to/lfj8p3jt0xp4", "lfj8p3jt0xp4"},
		{"https://vtbe.to/lfj8p3jt0xp4.html", "lfj8p3jt0xp4"},
		{"https://vtbe.to/e/lfj8p3jt0xp4", "lfj8p3jt0xp4"},
		{" lfj8p3jt0xp4 ", "lfj8p3jt0xp4"},
		{"short", ""},            // too short to be a code
		{"https://vtbe.to/", ""}, // no code at all
		{"not a url", ""},
	}
	for _, tt := range tests {
		got, ok := VTBECode(tt.ref)
		if tt.want == "" {
			if ok {
				t.Errorf("VTBECode(%q) = %q, want rejected", tt.ref, got)
			}
			continue
		}
		if !ok || got != tt.want {
			t.Errorf("VTBECode(%q) = %q,%v want %q,true", tt.ref, got, ok, tt.want)
		}
	}
}

// The real page shape, trimmed: the playlist sits in the player config with
// escaped slashes, and the token segment ends in ".urlset".
const vtbePage = `<!DOCTYPE html><html><head><title>Watch</title></head><body>
<div id="vplayer"></div>
<script>
  jwplayer("vplayer").setup({
    sources: [{file:"https:\/\/str12.vtube.network\/hls\/,x5s4yhdjljyki6cgaoe4xj5dnr3rojzlueqo5fiyuqwjoyejvy6uhsurjp4q,.urlset\/master.m3u8"}],
    image: "https:\/\/str12.vtube.network\/i\/01\/00001\/lfj8p3jt0xp4.jpg"
  });
</script></body></html>`

func TestVTBEResolve(t *testing.T) {
	var gotPath, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotUA = r.URL.Path, r.Header.Get("User-Agent")
		w.Write([]byte(vtbePage))
	}))
	defer srv.Close()

	res, err := (VTBE{Client: srv.Client()}).resolveFrom(
		context.Background(), srv.URL, "https://vtbe.to/embed-lfj8p3jt0xp4.html")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/embed-lfj8p3jt0xp4.html" {
		t.Errorf("fetched %q, want the embed page path", gotPath)
	}
	// Their edge serves a stub to non-browser agents.
	if !strings.Contains(gotUA, "Mozilla") {
		t.Errorf("User-Agent = %q, want a browser UA", gotUA)
	}
	if res.Kind != "hls" {
		t.Errorf("Kind = %q, want hls", res.Kind)
	}
	want := "https://str12.vtube.network/hls/,x5s4yhdjljyki6cgaoe4xj5dnr3rojzlueqo5fiyuqwjoyejvy6uhsurjp4q,.urlset/master.m3u8"
	if res.ManifestURL != want {
		t.Errorf("ManifestURL =\n  %q\nwant\n  %q", res.ManifestURL, want)
	}
	// Their CDN ignores Referer and answers ACAO * — claiming a required header
	// would make Probe test something the browser never sends.
	if len(res.Headers) != 0 {
		t.Errorf("Headers = %v, want none", res.Headers)
	}
}

// A deleted file still renders a page; the missing playlist is the signal.
func TestVTBEResolveDeletedFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><h1>File was deleted</h1></body></html>`))
	}))
	defer srv.Close()

	_, err := (VTBE{Client: srv.Client()}).resolveFrom(
		context.Background(), srv.URL, "lfj8p3jt0xp4")
	if !errors.Is(err, ErrUnresolvable) {
		t.Fatalf("err = %v, want ErrUnresolvable", err)
	}
}

func TestVTBEResolveBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := (VTBE{Client: srv.Client()}).resolveFrom(
		context.Background(), srv.URL, "lfj8p3jt0xp4")
	if !errors.Is(err, ErrUnresolvable) {
		t.Fatalf("err = %v, want ErrUnresolvable", err)
	}
}

func TestVTBEResolveRejectsJunkRef(t *testing.T) {
	if _, err := (VTBE{}).Resolve(context.Background(), "nope"); !errors.Is(err, ErrUnresolvable) {
		t.Fatalf("err = %v, want ErrUnresolvable", err)
	}
}

// The registry must know the provider name stored on content_links.
func TestVTBERegistered(t *testing.T) {
	if _, err := Default().Resolve(context.Background(), "vtbe", ""); err != nil &&
		strings.Contains(err.Error(), "no provider registered") {
		t.Fatal("vtbe is not in the default registry")
	}
}
