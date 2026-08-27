package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestRedactURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"no query", "/api/chat/messages", "/api/chat/messages"},
		{"harmless query kept", "/api/anime?page=3&limit=20", "/api/anime?page=3&limit=20"},
		{"token masked", "/api/chat/stream?token=eyJhbGciOi.secret.sig", "/api/chat/stream?token=REDACTED"},
		{"token masked among others", "/api/releases/7?token=abc&format=mp4", "/api/releases/7?format=mp4&token=REDACTED"},
		{"empty token still masked", "/api/chat/stream?token=", "/api/chat/stream?token=REDACTED"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			u, err := url.Parse(c.in)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got := redactURL(u); got != c.want {
				t.Errorf("redactURL(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// The property that actually matters: no matter what the query looks like, a
// token value must not survive into the logged string.
func TestRedactURLNeverLeaksTokenValue(t *testing.T) {
	const secret = "eyJhbGciOiJIUzI1NiJ9.PAYLOAD.SIGNATURE"
	for _, raw := range []string{
		"/x?token=" + secret,
		"/x?a=1&token=" + secret,
		"/x?token=" + secret + "&b=2",
		"/x?TOKEN=" + secret, // wrong case: not a param we inject, but check anyway
		"/x?%zz&token=" + secret, // unparseable query
	} {
		u, err := url.Parse(raw)
		if err != nil {
			continue
		}
		got := redactURL(u)
		if strings.Contains(got, "SIGNATURE") || strings.Contains(got, "PAYLOAD") {
			t.Errorf("token value leaked for %q: %q", raw, got)
		}
	}
}

// The chat stream asserts w.(http.Flusher) and returns 500 "Streaming
// unsupported" when it fails. Embedding the http.ResponseWriter INTERFACE
// promotes only its three methods, so any wrapper silently stops being a
// Flusher -- which is exactly how SSE broke when the metrics middleware landed.
// releases.go needs Unwrap for the same reason, via http.ResponseController.
func TestStatusRecorderKeepsWriterCapabilities(t *testing.T) {
	var w http.ResponseWriter = &statusRecorder{ResponseWriter: httptest.NewRecorder()}

	if _, ok := w.(http.Flusher); !ok {
		t.Error("statusRecorder is not an http.Flusher — SSE (chat stream) will 500")
	}
	if _, ok := w.(interface{ Unwrap() http.ResponseWriter }); !ok {
		t.Error("statusRecorder has no Unwrap — ResponseController cannot set deadlines")
	}
	if _, ok := w.(http.Hijacker); !ok {
		t.Error("statusRecorder is not an http.Hijacker — connection upgrades break")
	}
}
