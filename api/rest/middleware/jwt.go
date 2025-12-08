package middleware

import (
	"context"
	"net/http"

	"horizonx-server/internal/config"
	"horizonx-server/internal/core/auth"
)

type contextKey string

const UserIDKey contextKey = "user_id"

func JWT(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("access_token")
			if err != nil {
				http.Error(w, "Unauthorized: No token found", http.StatusUnauthorized)
				return
			}

			claims, err := auth.ValidateToken(cookie.Value, cfg.JWTSecret)
			if err != nil {
				http.Error(w, "Unauthorized: Invalid token", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), UserIDKey, claims["sub"])
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetUserID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(UserIDKey).(string)
	return id, ok
}
