package middleware

import (
	"net/http"

	"horizonx/internal/adapters/http/response"
	"horizonx/internal/config"
	"horizonx/internal/domain"
	"horizonx/internal/logger"
)

func JWT(cfg *config.Config, sessions domain.SessionStore, log logger.Logger) func(http.Handler) http.Handler {
	if log == nil {
		log = discardLogger{}
	}
	jw := response.NewJSONWriter(log)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("horizonx_access_token")
			if err != nil {
				jw.Write(w, http.StatusUnauthorized, &response.Response{Message: "Unauthorized: No token found"})
				return
			}

			claims, err := domain.ValidateToken(cookie.Value, cfg.JWTSecret)
			if err != nil {
				jw.Write(w, http.StatusUnauthorized, &response.Response{Message: "Unauthorized: Invalid token"})
				return
			}

			// Revocable sessions: a token is only valid while its session
			// exists server-side. Logout, password change, and admin kick
			// delete the session, killing the token before JWT expiry.
			if sessions != nil && claims.SessionID != "" {
				sess, err := sessions.Get(r.Context(), claims.SessionID)
				if err != nil || sess == nil {
					jw.Write(w, http.StatusUnauthorized, &response.Response{Message: "Unauthorized: Session revoked"})
					return
				}
				// Defensive: the session must belong to the token's user.
				if sess.UserID != claims.UserID {
					jw.Write(w, http.StatusUnauthorized, &response.Response{Message: "Unauthorized: Session mismatch"})
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
