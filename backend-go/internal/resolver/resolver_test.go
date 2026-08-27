package resolver

import (
	"context"
	"errors"
	"testing"
)

func TestDirect(t *testing.T) {
	tests := []struct {
		name     string
		ref      string
		wantKind string
		wantErr  bool
	}{
		{"hls manifest", "https://cdn.example/stream/master.m3u8", "hls", false},
		{"hls case-insensitive", "https://cdn.example/S/MASTER.M3U8", "hls", false},
		{"mp4", "https://cdn.example/v/ep1.mp4", "mp4", false},
		{"webm", "https://cdn.example/v/ep1.webm", "mp4", false},
		// the ref is the media, never a page — an HTML document handed to
		// <video> fails as an opaque CORS error, so reject it at the source
		{"no extension rejected", "https://cdn.example/v/ep1", "", true},
		{"host watch page rejected", "https://filemoon.org/en/vgKGxRKV38Qn/watch", "", true},
		{"host embed page rejected", "https://filemoon.sx/e/vgKGxRKV38Qn", "", true},
		{"http rejected", "http://cdn.example/master.m3u8", "", true},
		{"relative rejected", "/master.m3u8", "", true},
		{"garbage rejected", "not a url", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := Direct{}.Resolve(context.Background(), tt.ref)
			if tt.wantErr {
				if !errors.Is(err, ErrUnresolvable) {
					t.Errorf("err = %v, want ErrUnresolvable", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if res.ManifestURL != tt.ref || res.Kind != tt.wantKind {
				t.Errorf("got (%q, %q), want (%q, %q)", res.ManifestURL, res.Kind, tt.ref, tt.wantKind)
			}
		})
	}
}

func TestRegistry(t *testing.T) {
	r := Default()
	if _, err := r.Resolve(context.Background(), "direct", "https://cdn.example/a.m3u8"); err != nil {
		t.Errorf("direct via registry: %v", err)
	}
	if _, err := r.Resolve(context.Background(), "nosuchhost", "abc"); err == nil {
		t.Error("unregistered provider must error, not panic")
	}
	if _, ok := r.providers["filemoon"]; !ok {
		t.Error("filemoon must be registered by default")
	}
}
