package middleware

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics for the HTTP surface. Labelled by chi's ROUTE PATTERN, never the raw
// path: "/api/anime/{id}" is one series, "/api/anime/519" would be one series
// per title in the catalog. With 2,000+ anime that difference is the whole
// reason Prometheus setups fall over, so it is not an optimisation.
var (
	httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "animekage_http_requests_total",
		Help: "HTTP requests by method, route pattern and status class.",
	}, []string{"method", "route", "status"})

	httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "animekage_http_request_duration_seconds",
		Help: "HTTP request latency by method and route pattern.",
		// Tuned for this API: most reads answer in single-digit ms, while the
		// catalog walk and Jikan-backed syncs run into seconds. The default
		// buckets stop at 10s and would lose the tail that actually matters.
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
	}, []string{"method", "route"})

	httpInFlight = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "animekage_http_in_flight_requests",
		Help: "Requests currently being served.",
	})
)

// statusRecorder captures the status code, which http.ResponseWriter does not
// expose after the fact.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Write covers handlers that never call WriteHeader: net/http implies 200 on
// the first write, and without this they would all be recorded as 0.
func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

// Everything below re-exposes capabilities of the real writer that embedding
// http.ResponseWriter hides.
//
// This is not theoretical. Embedding the INTERFACE promotes only its three
// methods, so *statusRecorder stopped satisfying http.Flusher the moment this
// middleware was added -- and handler/chat.go asserts exactly that, so the
// SSE chat stream answered 500 "Streaming unsupported" on every connection.
// handler/releases.go has the quieter version of the same problem: without
// Unwrap, http.ResponseController cannot reach the real writer, so the deadline
// extension that lets a 3 GB video upload outlive the 30s WriteTimeout silently
// turns into ErrNotSupported.
//
// Any middleware that wraps a ResponseWriter owes it these three methods.

// Flush passes Server-Sent Events through to the client.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		// A stream that flushes before writing has still answered 200.
		if r.status == 0 {
			r.status = http.StatusOK
		}
		f.Flush()
	}
}

// Hijack keeps connection upgrades working through the recorder.
func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("metrics: underlying ResponseWriter is not a Hijacker")
	}
	return h.Hijack()
}

// Unwrap is what http.ResponseController follows to find the real writer
// (Go 1.20+), which is how SetReadDeadline/SetWriteDeadline reach it.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// Metrics records request count, latency and concurrency.
//
// Placed inside the router so chi has already matched a route by the time the
// deferred func runs — that is what makes RoutePattern() available.
func Metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		httpInFlight.Inc()
		rec := &statusRecorder{ResponseWriter: w}

		defer func() {
			httpInFlight.Dec()

			route := chi.RouteContext(r.Context()).RoutePattern()
			if route == "" {
				// No match: 404s on random paths would otherwise create one
				// label value per URL a scanner tries.
				route = "other"
			}
			status := rec.status
			if status == 0 {
				status = http.StatusOK
			}
			httpRequests.WithLabelValues(r.Method, route, strconv.Itoa(status)).Inc()
			httpDuration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
		}()

		next.ServeHTTP(rec, r)
	})
}
