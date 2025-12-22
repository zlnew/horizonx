// Package http
package http

import (
	"net/http"

	"horizonx-server/internal/adapters/http/middleware"
	"horizonx-server/internal/adapters/ws/agentws"
	"horizonx-server/internal/adapters/ws/userws"
	"horizonx-server/internal/config"
	"horizonx-server/internal/domain"
)

type RouterDeps struct {
	WsUser  *userws.Handler
	WsAgent *agentws.Handler

	Auth        *AuthHandler
	Job         *JobHandler
	Metrics     *MetricsHandler
	Server      *ServerHandler
	User        *UserHandler
	Application *ApplicationHandler
	Deployment  *DeploymentHandler

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

	// AGENT ENDPOINTS
	mux.Handle("POST /agent/metrics", agentStack.Then(http.HandlerFunc(deps.Metrics.Ingest)))
	mux.Handle("GET /agent/jobs", agentStack.Then(http.HandlerFunc(deps.Job.Index)))
	mux.Handle("POST /agent/jobs/{id}/start", agentStack.Then(http.HandlerFunc(deps.Job.Start)))
	mux.Handle("POST /agent/jobs/{id}/finish", agentStack.Then(http.HandlerFunc(deps.Job.Finish)))

	// SERVERS
	mux.Handle("GET /servers", userStack.Then(http.HandlerFunc(deps.Server.Index)))
	mux.Handle("POST /servers", userStack.Then(http.HandlerFunc(deps.Server.Store)))
	mux.Handle("PUT /servers/{id}", userStack.Then(http.HandlerFunc(deps.Server.Update)))
	mux.Handle("DELETE /servers/{id}", userStack.Then(http.HandlerFunc(deps.Server.Destroy)))
	mux.Handle("GET /servers/{id}/metrics/latest", userStack.Then(http.HandlerFunc(deps.Metrics.Latest)))

	// USERS
	mux.Handle("GET /users", userStack.Then(http.HandlerFunc(deps.User.Index)))
	mux.Handle("POST /users", userStack.Then(http.HandlerFunc(deps.User.Store)))
	mux.Handle("PUT /users/{id}", userStack.Then(http.HandlerFunc(deps.User.Update)))
	mux.Handle("DELETE /users/{id}", userStack.Then(http.HandlerFunc(deps.User.Destroy)))

	// JOBS (User-facing for monitoring)
	mux.Handle("GET /jobs", userStack.Then(http.HandlerFunc(deps.Job.Index)))
	mux.Handle("GET /jobs/{id}", userStack.Then(http.HandlerFunc(deps.Job.Show)))

	// APPLICATIONS
	mux.Handle("GET /applications", userStack.Then(http.HandlerFunc(deps.Application.Index)))
	mux.Handle("GET /applications/{id}", userStack.Then(http.HandlerFunc(deps.Application.Show)))
	mux.Handle("POST /applications", userStack.Then(http.HandlerFunc(deps.Application.Store)))
	mux.Handle("PUT /applications/{id}", userStack.Then(http.HandlerFunc(deps.Application.Update)))
	mux.Handle("DELETE /applications/{id}", userStack.Then(http.HandlerFunc(deps.Application.Destroy)))

	// APPLICATION ACTIONS
	mux.Handle("POST /applications/{id}/deploy", userStack.Then(http.HandlerFunc(deps.Application.Deploy)))
	mux.Handle("POST /applications/{id}/start", userStack.Then(http.HandlerFunc(deps.Application.Start)))
	mux.Handle("POST /applications/{id}/stop", userStack.Then(http.HandlerFunc(deps.Application.Stop)))
	mux.Handle("POST /applications/{id}/restart", userStack.Then(http.HandlerFunc(deps.Application.Restart)))

	// DEPLOYMENTS
	mux.Handle("GET /applications/{id}/deployments", userStack.Then(http.HandlerFunc(deps.Deployment.List)))
	mux.Handle("GET /applications/{id}/deployments/latest", userStack.Then(http.HandlerFunc(deps.Deployment.GetLatest)))
	mux.Handle("GET /deployments/{deployment_id}", userStack.Then(http.HandlerFunc(deps.Deployment.Show)))

	// ENVIRONMENT VARIABLES
	mux.Handle("GET /applications/{id}/env", userStack.Then(http.HandlerFunc(deps.Application.ListEnvVars)))
	mux.Handle("POST /applications/{id}/env", userStack.Then(http.HandlerFunc(deps.Application.AddEnvVar)))
	mux.Handle("PUT /applications/{id}/env/{key}", userStack.Then(http.HandlerFunc(deps.Application.UpdateEnvVar)))
	mux.Handle("DELETE /applications/{id}/env/{key}", userStack.Then(http.HandlerFunc(deps.Application.DeleteEnvVar)))

	// VOLUMES
	mux.Handle("GET /applications/{id}/volumes", userStack.Then(http.HandlerFunc(deps.Application.ListVolumes)))
	mux.Handle("POST /applications/{id}/volumes", userStack.Then(http.HandlerFunc(deps.Application.AddVolume)))
	mux.Handle("DELETE /applications/{id}/volumes/{volume_id}", userStack.Then(http.HandlerFunc(deps.Application.DeleteVolume)))

	return globalMw.Apply(mux)
}
