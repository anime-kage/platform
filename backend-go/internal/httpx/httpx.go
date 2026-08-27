// Package httpx holds the small JSON request/response helpers every handler uses.
// Response shapes mirror the old backend exactly: {"data": …}, {"error": …},
// {"data": …, "pagination": {…}} — the frontend must not notice the rewrite.
package httpx

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
)

// ClientIP returns the address to attribute a request to — what per-IP rate
// limiting counts and what password-reset requests are logged against.
//
// Two ways to get this wrong, and this codebase has had both:
//
//   - Read RemoteAddr only. Behind a reverse proxy every request arrives from
//     the proxy, so the whole internet collapses into one bucket and aggregate
//     traffic throttles the entire site.
//   - Read X-Forwarded-For (or X-Real-IP) unconditionally, which is what
//     chi's RealIP middleware did here until July 2026. Then every client picks
//     its own bucket by sending a header and per-IP limiting means nothing.
//
// So the header is read ONLY when the request actually came from a trusted
// proxy, and only the part of it that proxy is responsible for.
//
// The rightmost entry is the one the trusted hop observed and the client cannot
// forge, because a proxy APPENDS the peer it saw. (Caddy goes further: with its
// own trusted_proxies unset it discards a client-supplied X-Forwarded-For
// entirely and sends just the peer — but do not rely on that, since it does not
// sanitize X-Real-IP, which is exactly how the RealIP bug stayed exploitable
// behind the proxy.) We walk right-to-left past entries that are themselves
// trusted hops — which is what makes Caddy-behind-Cloudflare work once
// Cloudflare's ranges are in TRUSTED_PROXIES — and take the first that is not.
//
// X-Real-IP and True-Client-IP are deliberately NOT consulted: they carry a
// single value with no hop chain, so there is no way to tell the trustworthy
// part from the client-supplied part.
//
// With trusted empty (the default) this is exactly RemoteAddr, which is the
// right answer for a directly exposed server.
func ClientIP(r *http.Request, trusted []netip.Prefix) string {
	peer := stripPort(r.RemoteAddr)
	if len(trusted) == 0 || !ipTrusted(peer, trusted) {
		return peer
	}
	forwarded := r.Header.Values("X-Forwarded-For")
	var hops []string
	for _, h := range forwarded {
		for _, part := range strings.Split(h, ",") {
			if part = strings.TrimSpace(part); part != "" {
				hops = append(hops, part)
			}
		}
	}
	for i := len(hops) - 1; i >= 0; i-- {
		addr, err := netip.ParseAddr(stripPort(hops[i]))
		if err != nil {
			// Garbage in the header: stop trusting the rest of it.
			break
		}
		if !prefixesContain(trusted, addr) {
			return addr.String()
		}
	}
	return peer
}

func stripPort(addr string) string {
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}

func ipTrusted(ip string, trusted []netip.Prefix) bool {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	return prefixesContain(trusted, addr)
}

func prefixesContain(prefixes []netip.Prefix, addr netip.Addr) bool {
	addr = addr.Unmap()
	for _, p := range prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// ParseTrustedProxies reads a comma-separated list of CIDRs or bare IPs
// (`10.0.0.0/8, 172.16.0.0/12`) into prefixes for ClientIP.
func ParseTrustedProxies(raw string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "/") {
			p, err := netip.ParsePrefix(part)
			if err != nil {
				return nil, err
			}
			out = append(out, p.Masked())
			continue
		}
		addr, err := netip.ParseAddr(part)
		if err != nil {
			return nil, err
		}
		out = append(out, netip.PrefixFrom(addr.Unmap(), addr.Unmap().BitLen()))
	}
	return out, nil
}

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("encode response", "err", err)
	}
}

func Error(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, map[string]string{"error": msg})
}

// Internal logs the real error and returns the same generic message the old
// backend used, so clients never see internals.
func Internal(w http.ResponseWriter, action string, err error) {
	slog.Error(action, "err", err)
	Error(w, http.StatusInternalServerError, "Failed to "+action)
}

// Decode reads a JSON body into v; body size is capped to keep abuse cheap.
func Decode(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20) // 1 MB
	return json.NewDecoder(r.Body).Decode(v)
}

type Pagination struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"totalPages"`
}

func Paginated[T any](w http.ResponseWriter, data []T, page, limit, total int) {
	if data == nil {
		data = []T{}
	}
	totalPages := 0
	if limit > 0 {
		totalPages = (total + limit - 1) / limit
	}
	JSON(w, http.StatusOK, map[string]any{
		"data":       data,
		"pagination": Pagination{Page: page, Limit: limit, Total: total, TotalPages: totalPages},
	})
}

// IntParam parses a positive integer path/query value; ok=false means the
// caller should 400 like the old parseInt+isNaN guards did.
func IntParam(s string) (int, bool) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// QueryInt reads an integer query parameter clamped to [min, max], falling
// back to def when absent or malformed.
func QueryInt(r *http.Request, key string, def, min, max int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}
