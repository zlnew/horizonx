// Package http
package http

import (
	"net/http"

	"horizonx-server/internal/adapters/http/middleware"
	"horizonx-server/internal/adapters/ws"
	"horizonx-server/internal/config"
	"horizonx-server/internal/domain"
)

type RouterDeps struct {
	WsUser  *ws.UserHandler
	WsAgent *ws.AgentHandler
	Server  *ServerHandler
	Auth    *AuthHandler
	User    *UserHandler
	Job     *JobHandler

	ServerService domain.ServerService
}

func NewRouter(cfg *config.Config, deps *RouterDeps) http.Handler {
	mux := http.NewServeMux()

	globalMw := middleware.New()
	globalMw.Use(middleware.CORS(cfg))

	userStack := middleware.New()
	userStack.Use(middleware.JWT(cfg))
	userStack.Use(middleware.CSRF(cfg))

	agentStack := middleware.New()
	agentStack.Use(middleware.AgentAuth(deps.ServerService))

	// HEALTH
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	// WEBSOCKET
	mux.HandleFunc("GET /ws/user", deps.WsUser.Serve)
	mux.HandleFunc("GET /ws/agent", deps.WsAgent.Serve)

	// AUTH
	mux.HandleFunc("POST /auth/login", deps.Auth.Login)
	mux.Handle("POST /auth/logout", userStack.Then(http.HandlerFunc(deps.Auth.Logout)))

	// AGENT JOBS
	mux.Handle("GET /agent/jobs", agentStack.Then(http.HandlerFunc(deps.Job.Index)))
	mux.Handle("POST /agent/jobs/{id}/start", agentStack.Then(http.HandlerFunc(deps.Job.Start)))
	mux.Handle("POST /agent/jobs/{id}/finish", agentStack.Then(http.HandlerFunc(deps.Job.Finish)))

	// SERVERS
	mux.Handle("GET /servers", userStack.Then(http.HandlerFunc(deps.Server.Index)))
	mux.Handle("POST /servers", userStack.Then(http.HandlerFunc(deps.Server.Store)))
	mux.Handle("PUT /servers/{id}", userStack.Then(http.HandlerFunc(deps.Server.Update)))
	mux.Handle("DELETE /servers/{id}", userStack.Then(http.HandlerFunc(deps.Server.Destroy)))

	// USERS
	mux.Handle("GET /users", userStack.Then(http.HandlerFunc(deps.User.Index)))
	mux.Handle("POST /users", userStack.Then(http.HandlerFunc(deps.User.Store)))
	mux.Handle("PUT /users/{id}", userStack.Then(http.HandlerFunc(deps.User.Update)))
	mux.Handle("DELETE /users/{id}", userStack.Then(http.HandlerFunc(deps.User.Destroy)))

	return globalMw.Apply(mux)
}
