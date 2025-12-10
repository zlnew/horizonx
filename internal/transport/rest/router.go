// Package rest
package rest

import (
	"net/http"

	"horizonx-server/internal/config"
	"horizonx-server/internal/domain"
	"horizonx-server/internal/transport/rest/middleware"
	"horizonx-server/internal/transport/websocket"
)

type RouterDeps struct {
	WS      *websocket.Handler
	Server  *ServerHandler
	Metrics *MetricsHandler
	Auth    *AuthHandler
	User    *UserHandler

	ServerRepo domain.ServerRepository
}

func NewRouter(cfg *config.Config, deps *RouterDeps) http.Handler {
	mux := http.NewServeMux()

	globalMw := middleware.New()
	globalMw.Use(middleware.CORS(cfg))

	userStack := middleware.New()
	userStack.Use(middleware.JWT(cfg))
	userStack.Use(middleware.CSRF(cfg))

	agentStack := middleware.AgentAuth(deps.ServerRepo)

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	mux.HandleFunc("GET /ws", deps.WS.Serve)
	mux.HandleFunc("POST /auth/login", deps.Auth.Login)

	mux.Handle("POST /metrics/report", agentStack(http.HandlerFunc(deps.Metrics.Report)))

	mux.Handle("POST /auth/logout", userStack.Then(http.HandlerFunc(deps.Auth.Logout)))

	mux.Handle("GET /servers", userStack.Then(http.HandlerFunc(deps.Server.Index)))
	mux.Handle("POST /servers", userStack.Then(http.HandlerFunc(deps.Server.Store)))
	mux.Handle("PUT /servers/{id}", userStack.Then(http.HandlerFunc(deps.Server.Update)))
	mux.Handle("DELETE /servers/{id}", userStack.Then(http.HandlerFunc(deps.Server.Destroy)))

	mux.Handle("GET /users", userStack.Then(http.HandlerFunc(deps.User.Index)))
	mux.Handle("POST /users", userStack.Then(http.HandlerFunc(deps.User.Store)))
	mux.Handle("PUT /users/{id}", userStack.Then(http.HandlerFunc(deps.User.Update)))
	mux.Handle("DELETE /users/{id}", userStack.Then(http.HandlerFunc(deps.User.Destroy)))

	return globalMw.Apply(mux)
}
