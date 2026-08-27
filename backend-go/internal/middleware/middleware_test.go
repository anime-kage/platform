package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"animekage/backend/internal/auth"
	"animekage/backend/internal/httpx"
)

var testManager = auth.NewManager("test-secret", time.Hour)

func okHandler(t *testing.T, wantUser bool) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if wantUser && UserFrom(r) == nil {
			t.Error("handler reached without user in context")
		}
		w.WriteHeader(http.StatusOK)
	})
}

func TestRequireAuth(t *testing.T) {
	mw := RequireAuth(testManager, nil, nil)
	token, _ := testManager.Sign(7, "ana", "a@e.com", "user")

	tests := []struct {
		name, header string
		wantStatus   int
	}{
		{"no header", "", http.StatusUnauthorized},
		{"garbage token", "Bearer garbage", http.StatusUnauthorized},
		{"wrong scheme", "Basic " + token, http.StatusUnauthorized},
		{"valid", "Bearer " + token, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/", nil)
			if tt.header != "" {
				r.Header.Set("Authorization", tt.header)
			}
			mw(okHandler(t, tt.wantStatus == http.StatusOK)).ServeHTTP(w, r)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

func TestRequireAuthClaims(t *testing.T) {
	mw := RequireAuth(testManager, nil, nil)
	token, _ := testManager.Sign(7, "ana", "a@e.com", "admin")

	var got *auth.Claims
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = UserFrom(r)
	}))
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	h.ServeHTTP(httptest.NewRecorder(), r)

	if got == nil || got.UserID != 7 || got.Username != "ana" || got.Role != "admin" {
		t.Errorf("claims = %+v", got)
	}
}

func TestOptionalAuth(t *testing.T) {
	mw := OptionalAuth(testManager, nil)
	token, _ := testManager.Sign(7, "ana", "a@e.com", "user")

	tests := []struct {
		name, header string
		wantUser     bool
	}{
		{"guest passes with nil user", "", false},
		{"invalid token still passes as guest", "Bearer nope", false},
		{"valid token sets user", "Bearer " + token, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got *auth.Claims
			h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = UserFrom(r)
			}))
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/", nil)
			if tt.header != "" {
				r.Header.Set("Authorization", tt.header)
			}
			h.ServeHTTP(w, r)
			if w.Code != http.StatusOK {
				t.Errorf("status = %d, guests must pass", w.Code)
			}
			if (got != nil) != tt.wantUser {
				t.Errorf("user set = %v, want %v", got != nil, tt.wantUser)
			}
		})
	}
}

func TestRequireRole(t *testing.T) {
	authMW := RequireAuth(testManager, nil, nil)
	roleMW := RequireRole("admin", "translator")

	tests := []struct {
		role       string
		wantStatus int
	}{
		{"admin", http.StatusOK},
		{"translator", http.StatusOK},
		{"moderator", http.StatusForbidden},
		{"user", http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			token, _ := testManager.Sign(1, "u", "u@e.com", tt.role)
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/", nil)
			r.Header.Set("Authorization", "Bearer "+token)
			authMW(roleMW(okHandler(t, true))).ServeHTTP(w, r)
			if w.Code != tt.wantStatus {
				t.Errorf("role %q: status = %d, want %d", tt.role, w.Code, tt.wantStatus)
			}
		})
	}

	t.Run("unauthenticated is 401 not 403", func(t *testing.T) {
		w := httptest.NewRecorder()
		roleMW(okHandler(t, false)).ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		if w.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", w.Code)
		}
	})
}

func TestRateLimit(t *testing.T) {
	mw := RateLimit(60, 3, nil) // burst of 3, no trusted proxies
	h := mw(okHandler(t, false))

	do := func(ip string) int {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = ip + ":12345"
		h.ServeHTTP(w, r)
		return w.Code
	}

	for i := 1; i <= 3; i++ {
		if code := do("10.0.0.1"); code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200", i, code)
		}
	}
	if code := do("10.0.0.1"); code != http.StatusTooManyRequests {
		t.Errorf("4th request: status = %d, want 429", code)
	}
	// a different IP has its own bucket
	if code := do("10.0.0.2"); code != http.StatusOK {
		t.Errorf("other IP: status = %d, want 200", code)
	}
}

// Behind Caddy every request arrives from the proxy, so without trusted-proxy
// handling the whole site shares one bucket and any single visitor can 429
// everyone else. This is the regression test for that.
func TestRateLimitBucketsPerClientBehindAProxy(t *testing.T) {
	trusted, err := httpx.ParseTrustedProxies("172.16.0.0/12")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	h := RateLimit(60, 2, trusted)(okHandler(t, false))

	// all requests come from the proxy; only X-Forwarded-For tells them apart
	do := func(client string) int {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = "172.18.0.5:40000"
		r.Header.Set("X-Forwarded-For", client)
		h.ServeHTTP(w, r)
		return w.Code
	}

	for i := 1; i <= 2; i++ {
		if code := do("203.0.113.7"); code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200", i, code)
		}
	}
	if code := do("203.0.113.7"); code != http.StatusTooManyRequests {
		t.Errorf("3rd request from the same client: status = %d, want 429", code)
	}
	// the visitor next door must not be collateral damage — it gets its own
	// full burst even though the request reached us from the same proxy
	for i := 1; i <= 2; i++ {
		if code := do("203.0.113.8"); code != http.StatusOK {
			t.Fatalf("second client, request %d: status = %d, want 200 (shared bucket?)", i, code)
		}
	}

	// A client that prepends a forged hop is still counted as itself: Caddy
	// appends the peer it saw, so the RIGHTMOST entry is the one it cannot
	// forge. If we read the leftmost, "9.9.9.9" would be a fresh bucket and
	// per-IP limiting would be trivially bypassable.
	if code := do("9.9.9.9, 203.0.113.8"); code != http.StatusTooManyRequests {
		t.Errorf("spoofed leading hop: status = %d, want 429 (same bucket as 203.0.113.8)", code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	SecurityHeaders(okHandler(t, false)).ServeHTTP(w, httptest.NewRequest("GET", "/", nil))

	for header, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	} {
		if got := w.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	if w.Header().Get("Content-Security-Policy") == "" {
		t.Error("Content-Security-Policy not set")
	}
}
