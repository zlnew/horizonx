package http

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"time"

	"horizonx/internal/adapters/http/metrics"
)

// metricsMiddleware records every request against the Prometheus registry.
// P2-14: request counters + latency histogram.
func metricsMiddleware(reg *metrics.Registry) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			rec := &statusWriter{ResponseWriter: w}
			next.ServeHTTP(rec, r)

			status := rec.status
			if status == 0 {
				status = http.StatusOK
			}

			reg.ObserveRequest(
				r.Method,
				r.URL.Path,
				http.StatusText(status),
				time.Since(start).Seconds(),
			)
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

// Hijack lets WebSocket upgrades pass through the metrics middleware.
func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("underlying ResponseWriter does not implement http.Hijacker")
	}
	return hj.Hijack()
}
