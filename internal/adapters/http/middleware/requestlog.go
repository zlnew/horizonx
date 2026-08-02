// Package middleware
package middleware

import (
	"net/http"
	"time"

	"horizonx/internal/logger"
)

// statusRecorder captures the response status code written by the handler.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

// RequestLog logs every HTTP request with method, path, status and duration.
// P2-14: observability — structured access log via the existing slog logger.
func RequestLog(log logger.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			rec := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)

			status := rec.status
			if status == 0 {
				status = http.StatusOK
			}

			log.Info("http: request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", status,
				"duration_ms", time.Since(start).Milliseconds(),
				"remote", r.RemoteAddr,
			)
		})
	}
}
