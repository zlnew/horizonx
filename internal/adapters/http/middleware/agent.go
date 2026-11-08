package middleware

import (
	"context"
	"net/http"
	"strings"

	"horizonx/internal/adapters/http/response"
	"horizonx/internal/domain"
	"horizonx/internal/logger"
	"horizonx/internal/version"

	"github.com/google/uuid"
)

type serverIDKeyType int

const ServerIDKey serverIDKeyType = 0

// discardLogger keeps the envelope writer nil-safe when no logger is
// threaded in. Defined once here and shared by csrf.go and jwt.go.
type discardLogger struct{}

func (discardLogger) Debug(msg string, args ...any) {}
func (discardLogger) Info(msg string, args ...any)  {}
func (discardLogger) Warn(msg string, args ...any)  {}
func (discardLogger) Error(msg string, args ...any) {}

func Agent(svc domain.ServerService, log logger.Logger) Middleware {
	if log == nil {
		log = discardLogger{}
	}
	jw := response.NewJSONWriter(log)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if auth == "" {
				jw.Write(w, http.StatusUnauthorized, &response.Response{Message: "missing authorization header"})
				return
			}

			parts := strings.SplitN(auth, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				jw.Write(w, http.StatusUnauthorized, &response.Response{Message: "unauthorized"})
				return
			}

			serverID, secret, err := domain.ValidateAgentCredentials(parts[1])
			if err != nil {
				jw.Write(w, http.StatusUnauthorized, &response.Response{Message: err.Error()})
				return
			}

			server, err := svc.AuthorizeAgent(r.Context(), serverID, secret)
			if err != nil {
				jw.Write(w, http.StatusUnauthorized, &response.Response{Message: "invalid credentials"})
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
