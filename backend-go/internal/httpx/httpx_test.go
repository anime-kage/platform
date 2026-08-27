package httpx

import (
	"encoding/json"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestIntParam(t *testing.T) {
	tests := []struct {
		in   string
		want int
		ok   bool
	}{
		{"0", 0, true},
		{"42", 42, true},
		{"-1", 0, false},
		{"abc", 0, false},
		{"", 0, false},
		{"1.5", 0, false},
	}
	for _, tt := range tests {
		got, ok := IntParam(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("IntParam(%q) = (%d, %v), want (%d, %v)", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestQueryInt(t *testing.T) {
	tests := []struct {
		query string
		want  int
	}{
		{"limit=10", 10},
		{"limit=999", 50}, // clamped to max
		{"limit=0", 1},    // clamped to min
		{"limit=-5", 1},   // clamped to min
		{"limit=abc", 25}, // malformed → default
		{"", 25},          // absent → default
		{"other=3", 25},   // absent → default
	}
	for _, tt := range tests {
		r := httptest.NewRequest("GET", "/?"+tt.query, nil)
		if got := QueryInt(r, "limit", 25, 1, 50); got != tt.want {
			t.Errorf("QueryInt(%q) = %d, want %d", tt.query, got, tt.want)
		}
	}
}

func TestErrorShape(t *testing.T) {
	w := httptest.NewRecorder()
	Error(w, 404, "Anime not found")
	if w.Code != 404 {
		t.Errorf("status = %d, want 404", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body["error"] != "Anime not found" {
		t.Errorf(`body = %v, want {"error":"Anime not found"}`, body)
	}
}

func TestPaginated(t *testing.T) {
	t.Run("nil data becomes empty array", func(t *testing.T) {
		w := httptest.NewRecorder()
		Paginated[int](w, nil, 1, 25, 0)
		if !strings.Contains(w.Body.String(), `"data":[]`) {
			t.Errorf("nil data not serialized as []: %s", w.Body.String())
		}
	})
	t.Run("totalPages rounds up", func(t *testing.T) {
		w := httptest.NewRecorder()
		Paginated(w, []int{1, 2, 3}, 2, 25, 51)
		var body struct {
			Pagination Pagination `json:"pagination"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		want := Pagination{Page: 2, Limit: 25, Total: 51, TotalPages: 3}
		if body.Pagination != want {
			t.Errorf("pagination = %+v, want %+v", body.Pagination, want)
		}
	})
}

func TestDecodeSizeCap(t *testing.T) {
	big := `{"content":"` + strings.Repeat("x", 2<<20) + `"}` // 2 MB > 1 MB cap
	r := httptest.NewRequest("POST", "/", strings.NewReader(big))
	var v struct {
		Content string `json:"content"`
	}
	if err := Decode(r, &v); err == nil {
		t.Error("Decode accepted a body over the 1 MB cap")
	}

	r = httptest.NewRequest("POST", "/", strings.NewReader(`{"content":"ok"}`))
	if err := Decode(r, &v); err != nil || v.Content != "ok" {
		t.Errorf("Decode of small body failed: %v", err)
	}
}

func TestClientIP(t *testing.T) {
	trusted, err := ParseTrustedProxies("172.16.0.0/12, 10.0.0.1")
	if err != nil {
		t.Fatalf("ParseTrustedProxies: %v", err)
	}

	cases := []struct {
		name      string
		remote    string
		forwarded []string
		trusted   []netip.Prefix
		want      string
	}{
		// Directly exposed: the header must be ignored entirely, or anyone can
		// pick their own rate-limit bucket by sending a header.
		{"no trusted proxies ignores XFF", "203.0.113.9:1234",
			[]string{"9.9.9.9"}, nil, "203.0.113.9"},
		{"untrusted peer ignores XFF", "198.51.100.4:1234",
			[]string{"9.9.9.9"}, trusted, "198.51.100.4"},

		// Behind the proxy: the rightmost entry is the peer the proxy saw.
		{"trusted peer reads XFF", "172.18.0.5:40000",
			[]string{"203.0.113.7"}, trusted, "203.0.113.7"},
		{"rightmost wins over a forged prefix", "172.18.0.5:40000",
			[]string{"9.9.9.9, 203.0.113.7"}, trusted, "203.0.113.7"},
		{"walks past further trusted hops", "172.18.0.5:40000",
			[]string{"203.0.113.7, 10.0.0.1"}, trusted, "203.0.113.7"},
		{"repeated header fields are one chain", "172.18.0.5:40000",
			[]string{"9.9.9.9", "203.0.113.7"}, trusted, "203.0.113.7"},
		{"bare-IP prefix is a trusted hop", "10.0.0.1:40000",
			[]string{"203.0.113.7"}, trusted, "203.0.113.7"},

		// Degenerate input falls back to the peer rather than guessing.
		{"empty XFF falls back to peer", "172.18.0.5:40000",
			nil, trusted, "172.18.0.5"},
		{"garbage XFF falls back to peer", "172.18.0.5:40000",
			[]string{"not-an-ip"}, trusted, "172.18.0.5"},
		{"port in XFF is stripped", "172.18.0.5:40000",
			[]string{"203.0.113.7:55555"}, trusted, "203.0.113.7"},
		{"RemoteAddr without a port still parses", "172.18.0.5",
			nil, trusted, "172.18.0.5"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tc.remote
			for _, v := range tc.forwarded {
				r.Header.Add("X-Forwarded-For", v)
			}
			if got := ClientIP(r, tc.trusted); got != tc.want {
				t.Errorf("ClientIP = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseTrustedProxiesRejectsGarbage(t *testing.T) {
	for _, raw := range []string{"nope", "10.0.0.0/99", "10.0.0.0/", "1.2.3"} {
		if _, err := ParseTrustedProxies(raw); err == nil {
			t.Errorf("ParseTrustedProxies(%q) accepted an invalid value", raw)
		}
	}
	// empty and whitespace are valid: "trust nobody"
	for _, raw := range []string{"", "  ", " , "} {
		got, err := ParseTrustedProxies(raw)
		if err != nil || got != nil {
			t.Errorf("ParseTrustedProxies(%q) = %v, %v; want nil, nil", raw, got, err)
		}
	}
}
