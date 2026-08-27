package resolver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDoodRef(t *testing.T) {
	tests := []struct{ ref, host, code string }{
		{"https://playmogo.com/e/dckwz6z3qgbt", "playmogo.com", "dckwz6z3qgbt"},
		{"https://playmogo.com/d/dckwz6z3qgbt", "playmogo.com", "dckwz6z3qgbt"},
		{"playmogo.com/e/dckwz6z3qgbt", "playmogo.com", "dckwz6z3qgbt"},
		{"https://dood.video/e/dckwz6z3qgbt", "dood.video", "dckwz6z3qgbt"},
		{"dckwz6z3qgbt", "", ""},                     // bare code: which domain?
		{"https://playmogo.com/", "", ""},            // no code
		{"https://playmogo.com/e/short", "", ""},     // too short to be a code
		{"https://playmogo.com/x/dckwz6z3q", "", ""}, // unknown path shape
	}
	for _, tt := range tests {
		host, code, ok := doodRef(tt.ref)
		if tt.host == "" {
			if ok {
				t.Errorf("doodRef(%q) = %q,%q want rejected", tt.ref, host, code)
			}
			continue
		}
		if !ok || host != tt.host || code != tt.code {
			t.Errorf("doodRef(%q) = %q,%q,%v want %q,%q,true", tt.ref, host, code, ok, tt.host, tt.code)
		}
	}
}

// stubDood serves an embed page and a pass_md5 response off one test server.
func stubDood(t *testing.T, page, pass string) DoodStream {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/pass_md5/") {
			// the real endpoint only returns a usable prefix when the embed
			// page is the Referer — assert we send it
			if !strings.Contains(r.Header.Get("Referer"), "/e/") {
				http.Error(w, "hotlink", http.StatusForbidden)
				return
			}
			_, _ = w.Write([]byte(pass))
			return
		}
		_, _ = w.Write([]byte(page))
	}))
	t.Cleanup(srv.Close)
	return DoodStream{Client: &http.Client{Transport: rewriteHost{srv.Listener.Addr().String()}}}
}

const doodPage = `<video id="video_player"></video>` +
	`<script>$.get('/pass_md5/272025928-86/m72f1d0he0zbh77rhgczdfmx', function(data){});</script>`

func TestDoodStreamResolve(t *testing.T) {
	d := stubDood(t, doodPage, "https://cdn.example/abc/xyz~")
	res, err := d.Resolve(context.Background(), "https://playmogo.com/d/dckwz6z3qgbt")
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != "mp4" {
		t.Errorf("Kind = %q, want mp4 (they do not transcode)", res.Kind)
	}
	if !strings.HasPrefix(res.ManifestURL, "https://cdn.example/abc/xyz~") {
		t.Errorf("ManifestURL = %q, want the pass_md5 prefix", res.ManifestURL)
	}
	// prefix + ten random chars + the token from the pass_md5 path
	if !strings.Contains(res.ManifestURL, "token=m72f1d0he0zbh77rhgczdfmx") {
		t.Errorf("ManifestURL = %q, want the token appended", res.ManifestURL)
	}
	tail := strings.TrimPrefix(res.ManifestURL, "https://cdn.example/abc/xyz~")
	if got := strings.Index(tail, "?"); got != doodRandLen {
		t.Errorf("random tail = %d chars, want %d", got, doodRandLen)
	}
	// two resolves must not collide — the tail is regenerated each time
	res2, err := d.Resolve(context.Background(), "https://playmogo.com/d/dckwz6z3qgbt")
	if err != nil {
		t.Fatal(err)
	}
	if res2.ManifestURL == res.ManifestURL {
		t.Error("two resolves produced the same URL; the tail should be random")
	}
}

func TestDoodStreamResolveDeadFile(t *testing.T) {
	d := stubDood(t, `<html><title>File not found</title></html>`, "")
	_, err := d.Resolve(context.Background(), "https://playmogo.com/e/dckwz6z3qgbt")
	if !errors.Is(err, ErrUnresolvable) {
		t.Errorf("err = %v, want ErrUnresolvable", err)
	}
}

func TestDoodStreamResolvePlayerChanged(t *testing.T) {
	// file present, but the path we scrape is gone: an extractor bug, and it
	// must not be reported as a dead link
	d := stubDood(t, `<video id="video_player"></video>`, "")
	_, err := d.Resolve(context.Background(), "https://playmogo.com/e/dckwz6z3qgbt")
	if !errors.Is(err, ErrUnresolvable) {
		t.Fatalf("err = %v, want ErrUnresolvable", err)
	}
	if !strings.Contains(err.Error(), "extractor needs updating") {
		t.Errorf("err = %v, want it to name the extractor as the cause", err)
	}
}

func TestDoodStreamResolveBadPrefix(t *testing.T) {
	d := stubDood(t, doodPage, "not-a-url")
	_, err := d.Resolve(context.Background(), "https://playmogo.com/e/dckwz6z3qgbt")
	if !errors.Is(err, ErrUnresolvable) {
		t.Errorf("err = %v, want ErrUnresolvable", err)
	}
}

func TestRandAlnum(t *testing.T) {
	s := randAlnum(doodRandLen)
	if len(s) != doodRandLen {
		t.Fatalf("len = %d, want %d", len(s), doodRandLen)
	}
	for _, r := range s {
		if !strings.ContainsRune(alnum, r) {
			t.Errorf("randAlnum produced %q, which is not alphanumeric", r)
		}
	}
}
