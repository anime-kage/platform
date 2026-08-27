// Package middleware: auth context, role gates, per-IP rate limiting, and
// security headers. CORS comes from go-chi/cors, wired in the server.
package middleware

import (
	"log/slog"
	"context"
	"net/http"
	"net/netip"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"animekage/backend/internal/auth"
	"animekage/backend/internal/httpx"
)

type ctxKey int

const userKey ctxKey = 0

// UserFrom returns the authenticated claims, or nil for guests.
func UserFrom(r *http.Request) *auth.Claims {
	claims, _ := r.Context().Value(userKey).(*auth.Claims)
	return claims
}

// RoleLookup resolves a user's CURRENT role from storage. JWTs carry the role
// they were signed with, so without this a promotion/demotion only takes
// effect when the token is reissued — with it, role changes bite immediately.
type RoleLookup func(ctx context.Context, userID int) (string, error)

// refreshRole overwrites the claim's role with the live one. Lookup failures
// fall back to the signed claim: reads shouldn't 500 because the DB blinked,
// and the claim was legitimate at signing time.
func refreshRole(r *http.Request, claims *auth.Claims, lookup RoleLookup) *auth.Claims {
	if lookup == nil {
		return claims
	}
	if role, err := lookup(r.Context(), claims.UserID); err == nil && role != claims.Role {
		fresh := *claims
		fresh.Role = role
		return &fresh
	}
	return claims
}

// RequireAuth rejects requests without a valid token. Pass a RoleLookup so
// role gates see the database's role, not the (possibly stale) JWT claim;
// nil keeps the claim as-is.
// SeenRecorder marks a member as active. Nil disables the tracking entirely,
// which is what the tests and any caller that has not wired a repo get.
type SeenRecorder func(ctx context.Context, userID int) error

// seenThrottle keeps last_seen_at from becoming a write on every request. A
// member browsing hard would otherwise generate one UPDATE per image, poll and
// page load; the online count only needs minute resolution, so one write per
// user per minute is plenty and the rest are dropped in memory.
var seenThrottle = struct {
	sync.Mutex
	at map[int]time.Time
}{at: map[int]time.Time{}}

const seenInterval = time.Minute

func noteSeen(r *http.Request, userID int, rec SeenRecorder) {
	if rec == nil {
		return
	}
	now := time.Now()
	seenThrottle.Lock()
	last, ok := seenThrottle.at[userID]
	if ok && now.Sub(last) < seenInterval {
		seenThrottle.Unlock()
		return
	}
	seenThrottle.at[userID] = now
	if len(seenThrottle.at) > 10000 {
		// Bounded: drop anything older than the window rather than letting the
		// map grow with every account that ever logs in.
		for id, t := range seenThrottle.at {
			if now.Sub(t) > seenInterval {
				delete(seenThrottle.at, id)
			}
		}
	}
	seenThrottle.Unlock()

	// Detached context: the write must not be cancelled when the response is
	// written, and must never delay the response either.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := rec(ctx, userID); err != nil {
			slog.Warn("record last_seen", "userId", userID, "err", err)
		}
	}()
}

func RequireAuth(m *auth.Manager, lookup RoleLookup, seen SeenRecorder) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := auth.BearerToken(r.Header.Get("Authorization"))
			if token == "" {
				httpx.Error(w, http.StatusUnauthorized, "Authentication required")
				return
			}
			claims, err := m.Verify(token)
			if err != nil {
				httpx.Error(w, http.StatusUnauthorized, "Invalid or expired token")
				return
			}
			claims = refreshRole(r, claims, lookup)
			noteSeen(r, claims.UserID, seen)
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, claims)))
		})
	}
}

// TokenFromQuery promotes ?token= to the Authorization header when none is
// set. Media elements (<video src>, <track src>) cannot send headers, so the
// staging preview routes accept the JWT this way. Scope it tightly — a token
// in a URL lands in logs; it must never become the general auth path.
func TokenFromQuery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			if tok := r.URL.Query().Get("token"); tok != "" {
				r.Header.Set("Authorization", "Bearer "+tok)
			}
		}
		next.ServeHTTP(w, r)
	})
}

// OptionalAuth sets the user when a valid token is present, and stays silent
// otherwise — guest and logged-in requests share these routes.
func OptionalAuth(m *auth.Manager, lookup RoleLookup) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token := auth.BearerToken(r.Header.Get("Authorization")); token != "" {
				if claims, err := m.Verify(token); err == nil {
					claims = refreshRole(r, claims, lookup)
					r = r.WithContext(context.WithValue(r.Context(), userKey, claims))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireRole allows only the listed roles. Must run after RequireAuth.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(roles))
	for _, role := range roles {
		allowed[role] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := UserFrom(r)
			if user == nil {
				httpx.Error(w, http.StatusUnauthorized, "Authentication required")
				return
			}
			if !allowed[user.Role] {
				httpx.JSON(w, http.StatusForbidden, map[string]string{
					"error":   "Forbidden",
					"message": "You do not have permission to access this resource",
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RateLimit is a per-IP token bucket. Entries idle for 10 minutes are evicted
// by a background sweep so the map can't grow without bound.
//
// trusted is the reverse-proxy allowlist (cfg.TrustedProxies): behind Caddy,
// nothing but X-Forwarded-For can tell two visitors apart, and believing that
// header from anyone would let each of them pick a private bucket. Both
// failure modes are spelled out on httpx.ClientIP.
func RateLimit(perMinute int, burst int, trusted []netip.Prefix) func(http.Handler) http.Handler {
	type visitor struct {
		limiter *rate.Limiter
		seen    time.Time
	}
	var (
		mu       sync.Mutex
		visitors = map[string]*visitor{}
	)
	go func() {
		for range time.Tick(time.Minute) {
			mu.Lock()
			for ip, v := range visitors {
				if time.Since(v.seen) > 10*time.Minute {
					delete(visitors, ip)
				}
			}
			mu.Unlock()
		}
	}()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := httpx.ClientIP(r, trusted)
			mu.Lock()
			v, ok := visitors[ip]
			if !ok {
				v = &visitor{limiter: rate.NewLimiter(rate.Limit(float64(perMinute)/60), burst)}
				visitors[ip] = v
			}
			v.seen = time.Now()
			mu.Unlock()

			if !v.limiter.Allow() {
				httpx.Error(w, http.StatusTooManyRequests, "Too many requests, slow down")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders adds the baseline headers the old backend never set
// (code-review fix 1.4). CSP stays API-appropriate: nothing may render.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}
