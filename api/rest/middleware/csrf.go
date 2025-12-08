package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"time"

	"horizonx-server/internal/config"
)

func CSRF(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				if _, err := r.Cookie("csrf_token"); err != nil {
					setCSRFCookie(w, cfg)
				}
				next.ServeHTTP(w, r)
				return
			}

			cookie, err := r.Cookie("csrf_token")
			if err != nil {
				http.Error(w, "Missing CSRF cookie", http.StatusForbidden)
				return
			}

			tokenHeader := r.Header.Get("X-CSRF-Token")
			if tokenHeader == "" {
				http.Error(w, "Missing CSRF token header", http.StatusForbidden)
				return
			}

			if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(tokenHeader)) != 1 {
				http.Error(w, "Invalid CSRF token", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func setCSRFCookie(w http.ResponseWriter, cfg *config.Config) {
	token := generateRandomString(32)

	http.SetCookie(w, &http.Cookie{
		Name:     "csrf_token",
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(cfg.JWTExpiry),
		HttpOnly: false,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func generateRandomString(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}
