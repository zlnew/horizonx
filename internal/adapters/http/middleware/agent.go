package middleware

import (
	"context"
	"net/http"
	"strings"

	"horizonx/internal/domain"
	"horizonx/internal/logger"
	"horizonx/internal/version"

	"github.com/google/uuid"
)

type serverIDKeyType int

const ServerIDKey serverIDKeyType = 0

func Agent(svc domain.ServerService, log logger.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if auth == "" {
				http.Error(w, "missing authorization header", http.StatusUnauthorized)
				return
			}

			parts := strings.SplitN(auth, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			serverID, secret, err := domain.ValidateAgentCredentials(parts[1])
			if err != nil {
				http.Error(w, err.Error(), http.StatusUnauthorized)
				return
			}

			server, err := svc.AuthorizeAgent(r.Context(), serverID, secret)
			if err != nil {
				http.Error(w, "invalid credentials", http.StatusUnauthorized)
				return
			}

			// P2-18: agent version pinning — warn once per mismatch so a
			// stale agent can't silently drift from the server's behavior.
			if agentVersion := r.Header.Get(version.AgentVersionHeader); agentVersion != "" && agentVersion != version.Version {
				if log != nil {
					log.Warn("agent version mismatch",
						"server_id", server.ID,
						"agent_version", agentVersion,
						"server_version", version.Version,
					)
				}
			}

			ctx := context.WithValue(r.Context(), ServerIDKey, server.ID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetServerID(ctx context.Context) (uuid.UUID, bool) {
	v := ctx.Value(ServerIDKey)
	if v == nil {
		return uuid.Nil, false
	}

	id, ok := v.(uuid.UUID)
	if !ok {
		return uuid.Nil, false
	}

	return id, true
}
