package handler_test

// Per-IP rate limiting is only worth anything if the client cannot choose its
// own bucket. chi's RealIP middleware used to sit in front of the router and
// overwrite RemoteAddr from X-Real-IP / X-Forwarded-For with no trust check, so
// rotating one header defeated the login limiter entirely — and Caddy
// sanitizes X-Forwarded-For but NOT X-Real-IP, so the proxy did not cover for
// it. These tests are the lock on that door.

import (
	"fmt"
	"net/http"
	"testing"
)

// The auth limiter is 10/min with a burst of 10.
func TestRateLimitCannotBeEvadedWithSpoofedIPHeaders(t *testing.T) {
	srv, _ := newTestServer(t)

	// A client that claims a different X-Real-IP on every request. All of them
	// arrive from the same (untrusted-by-content) peer, so all of them must
	// land in the same bucket.
	hit := func(n int) int {
		req, err := http.NewRequest("POST", srv.URL+"/api/auth/login", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Real-IP", fmt.Sprintf("203.0.113.%d", n))
		req.Header.Set("True-Client-IP", fmt.Sprintf("198.51.100.%d", n))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	var got429 bool
	for i := 1; i <= 20; i++ {
		if hit(i) == http.StatusTooManyRequests {
			got429 = true
			break
		}
	}
	if !got429 {
		t.Error("20 requests with a fresh X-Real-IP each time were never rate limited — " +
			"the limiter is bypassable by header spoofing")
	}
}

// The flip side: X-Forwarded-For from a TRUSTED peer is how distinct clients
// legitimately get distinct buckets. Loopback is trusted in the test config
// (as the docker network is in production), so this must still work — a fix
// that made the limiter unforgeable by ignoring the header everywhere would
// throttle the whole site as one visitor.
func TestTrustedForwardedForStillSeparatesClients(t *testing.T) {
	srv, _ := newTestServer(t)

	spend := func(ip string) int {
		c := &client{t: t, base: srv.URL, ip: ip}
		var last int
		for i := 0; i < 12; i++ {
			status, _ := c.do("POST", "/api/auth/login",
				map[string]any{"email": "nobody@example.com", "password": "wrong-password"})
			last = status
			if status == http.StatusTooManyRequests {
				break
			}
		}
		return last
	}

	if got := spend("203.0.113.60"); got != http.StatusTooManyRequests {
		t.Fatalf("first client never hit the limit: last status %d", got)
	}
	// a different forwarded IP starts with a full bucket
	c := &client{t: t, base: srv.URL, ip: "203.0.113.61"}
	status, body := c.do("POST", "/api/auth/login",
		map[string]any{"email": "nobody@example.com", "password": "wrong-password"})
	if status == http.StatusTooManyRequests {
		t.Errorf("second client was throttled by the first client's traffic: %v", body)
	}
}
