package middleware

import (
	"encoding/json"
	"net/http"
)

// DemoGuard returns a middleware that rejects requests with 403 Forbidden
// when demoMode is active. Use this on destructive endpoints that should not
// be accessible to public demo visitors (e.g. password changes, server deletion,
// application creation/deletion, user management).
func DemoGuard(demoMode bool, message string) Middleware {
	if !demoMode {
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	if message == "" {
		message = "This action is disabled in public demo sandbox mode."
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"message": message,
			})
		})
	}
}
