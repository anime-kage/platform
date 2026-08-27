package resolver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFilemoonCode(t *testing.T) {
	tests := []struct {
		ref  string
		want string
	}{
		{"vgKGxRKV38Qn", "vgKGxRKV38Qn"},
		{"https://filemoon.org/en/vgKGxRKV38Qn/embed", "vgKGxRKV38Qn"},
		{"https://filemoon.org/en/vgKGxRKV38Qn/watch", "vgKGxRKV38Qn"},
		{"https://filemoon.org/e/vgKGxRKV38Qn", "vgKGxRKV38Qn"},
		{"https://filemoon.org/vgKGxRKV38Qn", "vgKGxRKV38Qn"},
		{" vgKGxRKV38Qn ", "vgKGxRKV38Qn"},
		{"short", ""},                    // too short to be a code
		{"https://filemoon.org/en/", ""}, // locale is not a code
		{"not a url", ""},
	}
	for _, tt := range tests {
		got, ok := FilemoonCode(tt.ref)
		if tt.want == "" {
			if ok {
				t.Errorf("FilemoonCode(%q) = %q, want rejected", tt.ref, got)
			}
			continue
		}
		if !ok || got != tt.want {
			t.Errorf("FilemoonCode(%q) = %q,%v want %q,true", tt.ref, got, ok, tt.want)
		}
	}
}

// stubFilemoon serves a page shaped like the real embed page. rewriting the
// host is what lets the test run offline.
func stubFilemoon(t *testing.T, page string) Filemoon {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(page))
	}))
	t.Cleanup(srv.Close)
	return Filemoon{Client: &http.Client{Transport: rewriteHost{srv.Listener.Addr().String()}}}
}

type rewriteHost struct{ addr string }

func (rw rewriteHost) RoundTrip(r *http.Request) (*http.Response, error) {
	r.URL.Scheme = "http"
	r.URL.Host = rw.addr
	return http.DefaultTransport.RoundTrip(r)
}

func TestFilemoonResolve(t *testing.T) {
	const code = "vgKGxRKV38Qn"
	page := `<div id="filemoon-player-box-` + code + `"></div>` +
		`<div data-hls-url="https://filemoon.org/en/` + code + `/stream?hls=1&amp;x=1"></div>`

	res, err := stubFilemoon(t, page).Resolve(context.Background(), code)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != "hls" {
		t.Errorf("Kind = %q, want hls", res.Kind)
	}
	// &amp; in the attribute must survive as a real & in the URL
	if want := "https://filemoon.org/en/" + code + "/stream?hls=1&x=1"; res.ManifestURL != want {
		t.Errorf("ManifestURL = %q, want %q", res.ManifestURL, want)
	}
	if !strings.Contains(res.Headers["Referer"], code) {
		t.Errorf("Referer = %q, want it to carry the embed URL", res.Headers["Referer"])
	}
}

func TestFilemoonResolveDeadFile(t *testing.T) {
	// a deleted file still renders a page — no player box means it is gone
	f := stubFilemoon(t, `<html><body>File not found</body></html>`)
	_, err := f.Resolve(context.Background(), "vgKGxRKV38Qn")
	if !errors.Is(err, ErrUnresolvable) {
		t.Errorf("err = %v, want ErrUnresolvable", err)
	}
}

func TestFilemoonResolvePlayerChanged(t *testing.T) {
	// the file is there but the attribute we scrape is gone: that is an
	// extractor bug, and it must not be reported as a dead link
	f := stubFilemoon(t, `<div id="filemoon-player-box-vgKGxRKV38Qn"></div>`)
	_, err := f.Resolve(context.Background(), "vgKGxRKV38Qn")
	if !errors.Is(err, ErrUnresolvable) {
		t.Fatalf("err = %v, want ErrUnresolvable", err)
	}
	if !strings.Contains(err.Error(), "extractor needs updating") {
		t.Errorf("err = %v, want it to name the extractor as the cause", err)
	}
}

func TestFilemoonResolveBadRef(t *testing.T) {
	_, err := Filemoon{}.Resolve(context.Background(), "nope")
	if !errors.Is(err, ErrUnresolvable) {
		t.Errorf("err = %v, want ErrUnresolvable", err)
	}
}
