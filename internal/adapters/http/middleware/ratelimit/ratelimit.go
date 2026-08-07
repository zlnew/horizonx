// Package ratelimit provides a small in-memory sliding-window rate limiter
// for auth endpoints (P1-10). Good for a single-instance control plane;
// swap for a Redis-backed limiter if HorizonX ever runs multi-instance.
package ratelimit

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

// Limiter enforces a per-key rate: at most limit requests in any window.
type Limiter struct {
	mu     sync.Mutex
	window time.Duration
	limit  int
	hits   map[string][]time.Time
}

func New(limit int, window time.Duration) *Limiter {
	return &Limiter{
		window: window,
		limit:  limit,
		hits:   make(map[string][]time.Time),
	}
}

// Allow reports whether a request for key is within the limit.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.window)

	hits := l.hits[key]

	// Drop entries outside the window.
	kept := hits[:0]
	for _, t := range hits {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	l.hits[key] = kept

	if len(kept) >= l.limit {
		return false
	}

	l.hits[key] = append(kept, now)
	return true
}

// ClientIP returns the remote address without the port. This is the
// pre-proxy address (the tunnel's IP) — use RealClientIP for the
// user-facing client when TRUST_PROXY is on.
func ClientIP(r *http.Request) string {
	host := r.RemoteAddr
	for i := len(host) - 1; i >= 0; i-- {
		if host[i] == ':' {
			return host[:i]
		}
	}
	return host
}

// RealClientIP resolves the actual client IP. When trustProxy is true it
// uses the FIRST X-Forwarded-For entry (set by the Cloudflare tunnel /
// tailscale serve in front of a 127.0.0.1-bound service) and falls back to
// RemoteAddr when absent. When false it returns the plain remote address —
// safe for direct exposure where spoofing XFF must not defeat the limiter.
func RealClientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			first := xff
			if i := strings.IndexByte(first, ','); i >= 0 {
				first = first[:i]
			}
			first = strings.TrimSpace(first)
			if first != "" {
				return first
			}
		}
	}
	return ClientIP(r)
}

// Middleware returns a handler that 429s keys over the limit.
func (l *Limiter) Middleware(keyFunc func(r *http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := keyFunc(r)
			if !l.Allow(key) {
				http.Error(w, "too many requests", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
