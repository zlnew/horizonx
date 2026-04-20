package middleware

import (
	"context"
	"net/http"

	"horizonx/internal/config"
	"horizonx/internal/domain"
)

type userContextKeyType struct{}

var userContextKey = userContextKeyType{}

func JWT(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("access_token")
			if err != nil {
				http.Error(w, "Unauthorized: No token found", http.StatusUnauthorized)
				return
			}

			claims, err := domain.ValidateToken(cookie.Value, cfg.JWTSecret)
			if err != nil {
				http.Error(w, "Unauthorized: Invalid token", http.StatusUnauthorized)
				return
			}

			userCtx := domain.UserContext{
				ID:   claims.UserID,
				Role: claims.Role,
			}

			ctx := domain.SetUserContext(r.Context(), userCtx)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetUser(ctx context.Context) (domain.UserContext, bool) {
	user, ok := ctx.Value(userContextKey).(domain.UserContext)
	if !ok {
		return domain.UserContext{}, false
	}

	return user, true
}
