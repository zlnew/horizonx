package http

import (
	"net/http"

	"horizonx/internal/adapters/http/middleware/ratelimit"
	"horizonx/internal/config"
)

// clientIP resolves the real client IP for session metadata, honoring the
// TRUST_PROXY setting (see ratelimit.RealClientIP).
func clientIP(r *http.Request, cfg *config.Config) string {
	trustProxy := cfg == nil || cfg.TrustProxy
	return ratelimit.RealClientIP(r, trustProxy)
}
