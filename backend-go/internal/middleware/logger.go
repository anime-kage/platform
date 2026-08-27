package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Query parameters that must never reach a log file.
//
// `token` is the session JWT. Two routes accept it in the URL because the
// browser API cannot do otherwise -- EventSource (chat stream) and
// <video>/<track> (release preview) send no Authorization header. chi's stock
// Logger prints the whole request URI, so every one of those requests wrote a
// live credential into the container log, where it stayed for the token's full
// 7-day life. The claims carry no scope, so that string is the whole account,
// not just the endpoint it leaked from.
var redactedParams = []string{"token"}

// redactURL returns the request URI with sensitive parameters masked. The query
// is otherwise preserved, since ?page= and ?limit= are worth having in a log.
func redactURL(u *url.URL) string {
	if u.RawQuery == "" {
		return u.Path
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		// Unparseable query: drop it wholesale rather than risk printing a
		// token that just happened not to parse.
		return u.Path + "?<unparseable>"
	}
	// Case-insensitive on purpose. Query().Get is case-sensitive, so ?TOKEN=
	// does not authenticate today -- but a sanitiser should hide anything that
	// LOOKS like a credential, not only the spelling that currently works.
	redacted := false
	for name := range q {
		for _, p := range redactedParams {
			if strings.EqualFold(name, p) {
				q.Set(name, "REDACTED")
				redacted = true
			}
		}
	}
	if !redacted {
		return u.Path + "?" + u.RawQuery
	}
	return u.Path + "?" + q.Encode()
}

// Logger is a drop-in replacement for chi's Logger that redacts credentials
// from the URL. Same shape of line, minus the part that must not be kept.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}

		defer func() {
			status := rec.status
			if status == 0 {
				status = http.StatusOK
			}
			slog.Info("http",
				"method", r.Method,
				"path", redactURL(r.URL),
				"status", status,
				"from", r.RemoteAddr,
				"took", fmt.Sprintf("%.3fms", float64(time.Since(start).Nanoseconds())/1e6),
			)
		}()

		next.ServeHTTP(rec, r)
	})
}
