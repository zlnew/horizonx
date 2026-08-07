package middleware

import (
	"context"
	"net/http"

	"horizonx/internal/config"
	"horizonx/internal/domain"
)

type userContextKeyType struct{}

var userContextKey = userContextKeyType{}

func JWT(cfg *config.Config, sessions domain.SessionStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("horizonx_access_token")
			if err != nil {
				http.Error(w, "Unauthorized: No token found", http.StatusUnauthorized)
				return
			}

			claims, err := domain.ValidateToken(cookie.Value, cfg.JWTSecret)
			if err != nil {
				http.Error(w, "Unauthorized: Invalid token", http.StatusUnauthorized)
				return
			}

			// Revocable sessions: a token is only valid while its session
			// exists server-side. Logout, password change, and admin kick
			// delete the session, killing the token before JWT expiry.
			if sessions != nil && claims.SessionID != "" {
				sess, err := sessions.Get(r.Context(), claims.SessionID)
				if err != nil || sess == nil {
					http.Error(w, "Unauthorized: Session revoked", http.StatusUnauthorized)
					return
				}
				// Defensive: the session must belong to the token's user.
				if sess.UserID != claims.UserID {
					http.Error(w, "Unauthorized: Session mismatch", http.StatusUnauthorized)
					return
				}
			}

			userCtx := domain.UserContext{
				ID:        claims.UserID,
				Role:      claims.Role,
				SessionID: claims.SessionID,
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
