// Package ratelimit provides a small in-memory sliding-window rate limiter
// for auth endpoints (P1-10). Good for a single-instance control plane;
// swap for a Redis-backed limiter if HorizonX ever runs multi-instance.
package ratelimit

import (
	"net/http"
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

// ClientIP returns the remote address without the port.
func ClientIP(r *http.Request) string {
	host := r.RemoteAddr
	for i := len(host) - 1; i >= 0; i-- {
		if host[i] == ':' {
			return host[:i]
		}
	}
	return host
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
