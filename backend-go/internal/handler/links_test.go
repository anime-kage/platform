package handler

import "testing"

func TestValidateHostingURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		allowed []string
		wantOK  bool
	}{
		{"plain https", "https://filemoon.sx/e/abc123", nil, true},
		{"subdomain", "https://cdn.videohost.com/v/1", nil, true},
		{"http rejected", "http://filemoon.sx/e/abc123", nil, false},
		{"javascript scheme", "javascript:alert(1)", nil, false},
		{"relative", "/e/abc123", nil, false},
		{"garbage", "not a url", nil, false},
		{"ip literal", "https://93.184.216.34/e/x", nil, false},
		{"ipv6 literal", "https://[::1]/e/x", nil, false},
		{"localhost", "https://localhost/e/x", nil, false},
		{"single-label host", "https://backend/e/x", nil, false},
		{"internal suffix", "https://db.internal/e/x", nil, false},
		{"credentials", "https://user:pw@filemoon.sx/e/x", nil, false},

		{"on allowlist", "https://filemoon.sx/e/x", []string{"filemoon.sx"}, true},
		{"allowlist subdomain", "https://cdn.filemoon.sx/e/x", []string{"filemoon.sx"}, true},
		{"off allowlist", "https://evil.com/e/x", []string{"filemoon.sx"}, false},
		{"suffix trick rejected", "https://notfilemoon.sx/e/x", []string{"filemoon.sx"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateHostingURL(tt.url, tt.allowed)
			if (err == nil) != tt.wantOK {
				t.Errorf("validateHostingURL(%q, %v) = %v, want ok=%v", tt.url, tt.allowed, err, tt.wantOK)
			}
		})
	}
}

func TestFileHostEmbed(t *testing.T) {
	const embed = "https://filemoon.org/en/vgKGxRKV38Qn/embed"
	tests := []struct{ in, want string }{
		// the pages you land on after uploading → the host's own embed form
		{"https://filemoon.org/en/vgKGxRKV38Qn/watch", embed},
		{"https://filemoon.org/en/vgKGxRKV38Qn/file", embed},
		{"https://filemoon.org/vgKGxRKV38Qn", embed},
		{"https://filemoon.org/d/vgKGxRKV38Qn", embed},
		{"https://filemoon.org/e/vgKGxRKV38Qn", embed},
		// already the embed URL — idempotent
		{embed, embed},
		// a non-English dashboard keeps its locale
		{"https://filemoon.org/ro/vgKGxRKV38Qn/watch", "https://filemoon.org/ro/vgKGxRKV38Qn/embed"},
		// the host is preserved: their domains are not interchangeable
		{"https://filemoon.sx/vgKGxRKV38Qn", "https://filemoon.sx/en/vgKGxRKV38Qn/embed"},
		// unknown host: never guess a path
		{"https://player.example.com/en/vgKGxRKV38Qn/watch", "https://player.example.com/en/vgKGxRKV38Qn/watch"},
		// known host, no file code to find: leave it alone
		{"https://filemoon.org/", "https://filemoon.org/"},
		// a host with a different embed shape (DoodStream family): /e/{code},
		// and the download page is what their dashboard shows you
		{"https://playmogo.com/d/dckwz6z3qgbt", "https://playmogo.com/e/dckwz6z3qgbt"},
		{"https://playmogo.com/e/dckwz6z3qgbt", "https://playmogo.com/e/dckwz6z3qgbt"},
	}
	for _, tt := range tests {
		if got := fileHostEmbed(tt.in); got != tt.want {
			t.Errorf("fileHostEmbed(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
