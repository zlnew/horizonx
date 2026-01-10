// Package http
package http

import (
	"net/http"

	"horizonx/internal/adapters/http/middleware"
	"horizonx/internal/adapters/ws/agentws"
	"horizonx/internal/adapters/ws/userws"
	"horizonx/internal/config"
	"horizonx/internal/domain"
)

type RouterDeps struct {
	WsUser  *userws.Handler
	WsAgent *agentws.Handler

	Auth        *AuthHandler
	Account     *AccountHandler
	User        *UserHandler
	Server      *ServerHandler
	Log         *LogHandler
	Job         *JobHandler
	Metrics     *MetricsHandler
	Application *ApplicationHandler
	Deployment  *DeploymentHandler

	RoleService   domain.RoleService
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
	agentStack.Use(middleware.Agent(deps.ServerService))

	metricsReadStack := userStack.Extend(middleware.Permission(deps.RoleService, domain.PermMetricsRead))

	serverReadStack := userStack.Extend(middleware.Permission(deps.RoleService, domain.PermServerRead))
	serverWriteStack := userStack.Extend(middleware.Permission(deps.RoleService, domain.PermServerWrite))

	memberReadStack := userStack.Extend(middleware.Permission(deps.RoleService, domain.PermMemberRead))
	memberWriteStack := userStack.Extend(middleware.Permission(deps.RoleService, domain.PermMemberWrite))

	appReadStack := userStack.Extend(middleware.Permission(deps.RoleService, domain.PermAppRead))
	appWriteStack := userStack.Extend(middleware.Permission(deps.RoleService, domain.PermAppWrite))

	// HEALTH
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	// WEBSOCKET
	mux.HandleFunc("GET /ws/user", deps.WsUser.Serve)
	mux.HandleFunc("GET /ws/agent", deps.WsAgent.Serve)

	// AUTH
	mux.Handle("GET /auth/user", userStack.ThenFunc(deps.Auth.User))
	mux.Handle("POST /auth/login", http.HandlerFunc(deps.Auth.Login))
	mux.Handle("POST /auth/logout", userStack.ThenFunc(deps.Auth.Logout))

	// AGENT ENDPOINTS
	mux.Handle("POST /agent/logs", agentStack.ThenFunc(deps.Log.Store))
	mux.Handle("GET /agent/jobs", agentStack.ThenFunc(deps.Job.Pending))
	mux.Handle("POST /agent/jobs/{id}/start", agentStack.ThenFunc(deps.Job.Start))
	mux.Handle("POST /agent/jobs/{id}/finish", agentStack.ThenFunc(deps.Job.Finish))
	mux.Handle("POST /agent/metrics", agentStack.ThenFunc(deps.Metrics.Ingest))
	mux.Handle("POST /agent/applications/health", agentStack.ThenFunc(deps.Application.ReportHealth))
	mux.Handle("POST /agent/deployments/{id}/commit-info", agentStack.ThenFunc(deps.Deployment.UpdateCommitInfo))

	// LOGS
	mux.Handle("GET /logs", userStack.ThenFunc(deps.Log.Index))

	// JOBS
	mux.Handle("GET /jobs", userStack.ThenFunc(deps.Job.Index))
	mux.Handle("GET /jobs/{id}", userStack.ThenFunc(deps.Job.Show))

	// SERVERS
	mux.Handle("GET /servers", serverReadStack.ThenFunc(deps.Server.Index))
	mux.Handle("POST /servers", serverWriteStack.ThenFunc(deps.Server.Store))
	mux.Handle("PUT /servers/{id}", serverWriteStack.ThenFunc(deps.Server.Update))
	mux.Handle("DELETE /servers/{id}", serverWriteStack.ThenFunc(deps.Server.Destroy))

	// SERVER METRICS
	mux.Handle("GET /servers/{id}/metrics/latest", metricsReadStack.ThenFunc(deps.Metrics.Latest))
	mux.Handle("GET /servers/{id}/metrics/cpu-usage-history", metricsReadStack.ThenFunc(deps.Metrics.CPUUsageHistory))
	mux.Handle("GET /servers/{id}/metrics/net-speed-history", metricsReadStack.ThenFunc(deps.Metrics.NetSpeedHistory))

	// ACCOUNT
	mux.Handle("POST /account/profile", userStack.ThenFunc(deps.Account.Profile))
	mux.Handle("POST /account/password", userStack.ThenFunc(deps.Account.Password))

	// USERS
	mux.Handle("GET /users", memberReadStack.ThenFunc(deps.User.Index))
	mux.Handle("POST /users", memberWriteStack.ThenFunc(deps.User.Store))
	mux.Handle("PUT /users/{id}", memberWriteStack.ThenFunc(deps.User.Update))
	mux.Handle("DELETE /users/{id}", memberWriteStack.ThenFunc(deps.User.Destroy))

	// APPLICATIONS
	mux.Handle("GET /applications", appReadStack.ThenFunc(deps.Application.Index))
	mux.Handle("GET /applications/{id}", appReadStack.ThenFunc(deps.Application.Show))
	mux.Handle("POST /applications", appWriteStack.ThenFunc(deps.Application.Store))
	mux.Handle("PUT /applications/{id}", appWriteStack.ThenFunc(deps.Application.Update))
	mux.Handle("DELETE /applications/{id}", appWriteStack.ThenFunc(deps.Application.Destroy))

	// APPLICATION ACTIONS
	mux.Handle("POST /applications/{id}/deploy", appWriteStack.ThenFunc(deps.Application.Deploy))
	mux.Handle("POST /applications/{id}/start", appWriteStack.ThenFunc(deps.Application.Start))
	mux.Handle("POST /applications/{id}/stop", appWriteStack.ThenFunc(deps.Application.Stop))
	mux.Handle("POST /applications/{id}/restart", appWriteStack.ThenFunc(deps.Application.Restart))

	// DEPLOYMENTS
	mux.Handle("GET /applications/{id}/deployments", appReadStack.ThenFunc(deps.Deployment.Index))
	mux.Handle("GET /applications/{id}/deployments/{deployment_id}", appReadStack.ThenFunc(deps.Deployment.Show))

	// ENVIRONMENT VARIABLES
	mux.Handle("POST /applications/{id}/env", appWriteStack.ThenFunc(deps.Application.AddEnvVar))
	mux.Handle("PUT /applications/{id}/env/{key}", appWriteStack.ThenFunc(deps.Application.UpdateEnvVar))
	mux.Handle("DELETE /applications/{id}/env/{key}", appWriteStack.ThenFunc(deps.Application.DeleteEnvVar))

	return globalMw.Apply(mux)
}
