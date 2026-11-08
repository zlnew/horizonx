package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"time"

	"horizonx/internal/adapters/http/response"
	"horizonx/internal/config"
	"horizonx/internal/logger"
)

func CSRF(cfg *config.Config, log logger.Logger) func(http.Handler) http.Handler {
	if log == nil {
		log = discardLogger{}
	}
	jw := response.NewJSONWriter(log)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				if _, err := r.Cookie("horizonx_csrf_token"); err != nil {
					setCSRFCookie(w, cfg)
				}
				next.ServeHTTP(w, r)
				return
			}

			cookie, err := r.Cookie("horizonx_csrf_token")
			if err != nil {
				jw.Write(w, http.StatusForbidden, &response.Response{Message: "Missing CSRF cookie"})
				return
			}

			tokenHeader := r.Header.Get("X-CSRF-Token")
			if tokenHeader == "" {
				jw.Write(w, http.StatusForbidden, &response.Response{Message: "Missing CSRF token header"})
				return
			}

			if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(tokenHeader)) != 1 {
				jw.Write(w, http.StatusForbidden, &response.Response{Message: "Invalid CSRF token"})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func setCSRFCookie(w http.ResponseWriter, cfg *config.Config) {
	token := generateRandomString(32)

	http.SetCookie(w, &http.Cookie{
		Name:     "horizonx_csrf_token",
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
