// Package http
package http

import (
	"encoding/json"
	"net/http"
	"time"

	"horizonx/internal/adapters/http/metrics"
	"horizonx/internal/adapters/http/middleware"
	"horizonx/internal/adapters/http/middleware/ratelimit"
	"horizonx/internal/adapters/ws/agentws"
	"horizonx/internal/adapters/ws/userws"
	"horizonx/internal/config"
	"horizonx/internal/domain"
	"horizonx/internal/logger"
)

type RouterDeps struct {
	WsUser       *userws.Handler
	WsAgent      *agentws.Handler
	WsAgentRoute *agentws.Router

	Auth        *AuthHandler
	Account     *AccountHandler
	User        *UserHandler
	Server      *ServerHandler
	Log         *LogHandler
	Job         *JobHandler
	Metrics     *MetricsHandler
	Application *ApplicationHandler
	Deployment  *DeploymentHandler
	AuditLog    *AuditLogHandler
	Settings    *SettingsHandler
	Alert       *AlertHandler

	SessionStore domain.SessionStore

	RoleService   domain.RoleService
	ServerService domain.ServerService

	MetricsRegistry *metrics.Registry
	Logger          logger.Logger
}

func NewRouter(cfg *config.Config, deps *RouterDeps) http.Handler {
	mux := http.NewServeMux()

	globalMw := middleware.New()
	globalMw.Use(middleware.CORS(cfg))
	if deps.Logger != nil {
		globalMw.Use(middleware.RequestLog(deps.Logger))
	}
	if deps.MetricsRegistry != nil {
		globalMw.Use(metricsMiddleware(deps.MetricsRegistry))
	}

	userStack := middleware.New()
	userStack.Use(middleware.JWT(cfg, deps.SessionStore))
	userStack.Use(middleware.CSRF(cfg))

	agentStack := middleware.New()
	agentStack.Use(middleware.Agent(deps.ServerService, deps.Logger))

	metricsReadStack := userStack.Extend(middleware.Permission(deps.RoleService, domain.PermMetricsRead))

	demoGuard := middleware.DemoGuard(cfg.DemoMode, "This action is disabled in public demo sandbox mode.")

	serverReadStack := userStack.Extend(middleware.Permission(deps.RoleService, domain.PermServerRead))
	serverWriteStack := userStack.Extend(middleware.Permission(deps.RoleService, domain.PermServerWrite))

	memberReadStack := userStack.Extend(middleware.Permission(deps.RoleService, domain.PermMemberRead))
	memberWriteStack := userStack.Extend(middleware.Permission(deps.RoleService, domain.PermMemberWrite), demoGuard)

	appReadStack := userStack.Extend(middleware.Permission(deps.RoleService, domain.PermAppRead))
	appWriteStack := userStack.Extend(middleware.Permission(deps.RoleService, domain.PermAppWrite))

	alertReadStack := userStack.Extend(middleware.Permission(deps.RoleService, domain.PermAlertRead))
	alertWriteStack := userStack.Extend(middleware.Permission(deps.RoleService, domain.PermAlertWrite))

	// P1-10: brute-force guard on the public login endpoint — 5 attempts per
	// IP per minute, then HTTP 429. With TRUST_PROXY the key is the real
	// client IP from X-Forwarded-For (Cloudflare tunnel); otherwise the
	// tunnel's address.
	loginLimiter := ratelimit.New(5, time.Minute)
	loginStack := middleware.New().Use(loginLimiter.Middleware(func(r *http.Request) string {
		return ratelimit.RealClientIP(r, cfg.TrustProxy)
	}))

	// HEALTH
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	// SYSTEM & RUNTIME CONFIG (public endpoint for client/dashboard features)
	mux.HandleFunc("GET /config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"demo_mode": cfg.DemoMode,
		})
	})
	mux.HandleFunc("GET /auth/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"demo_mode": cfg.DemoMode,
		})
	})

	// P2-14: Prometheus scrape endpoint. Public (no auth) — exposes only
	// operational counters, no sensitive data. Refresh gauges before serving.
	if deps.MetricsRegistry != nil {
		mux.Handle("GET /metrics", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			deps.MetricsRegistry.Refresh()
			deps.MetricsRegistry.Handler().ServeHTTP(w, r)
		}))
	}

	// WEBSOCKET
	mux.HandleFunc("GET /ws/user", deps.WsUser.Serve)
	mux.HandleFunc("GET /ws/agent", deps.WsAgent.Serve)

	// AUTH
	mux.Handle("GET /auth/user", userStack.ThenFunc(deps.Auth.User))
	mux.Handle("POST /auth/login", loginStack.ThenFunc(deps.Auth.Login))
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

	// P2-17: queue depth summary (no pagination — tiny fixed-size response).
	mux.Handle("GET /jobs/summary", userStack.ThenFunc(deps.Job.Summary))

	// v0.3.13 Track C: re-queue a failed/expired job with its original payload.
	mux.Handle("POST /jobs/{id}/retry", userStack.ThenFunc(deps.Job.Retry))

	// SERVERS
	mux.Handle("GET /servers", serverReadStack.ThenFunc(deps.Server.Index))
	mux.Handle("POST /servers", serverWriteStack.Extend(demoGuard).ThenFunc(deps.Server.Store))
	mux.Handle("PUT /servers/{id}", serverWriteStack.ThenFunc(deps.Server.Update))
	mux.Handle("DELETE /servers/{id}", serverWriteStack.Extend(demoGuard).ThenFunc(deps.Server.Destroy))
	mux.Handle("POST /servers/{id}/rotate-secret", serverWriteStack.Extend(demoGuard).ThenFunc(deps.Server.RotateSecret))
	mux.Handle("POST /servers/{id}/ping", serverWriteStack.ThenFunc(deps.Server.Ping))

	// SERVER METRICS
	mux.Handle("GET /servers/{id}/metrics/latest", metricsReadStack.ThenFunc(deps.Metrics.Latest))
	mux.Handle("GET /servers/{id}/metrics/cpu-usage-history", metricsReadStack.ThenFunc(deps.Metrics.CPUUsageHistory))
	mux.Handle("GET /servers/{id}/metrics/net-speed-history", metricsReadStack.ThenFunc(deps.Metrics.NetSpeedHistory))

	// ACCOUNT
	mux.Handle("POST /account/profile", userStack.Extend(demoGuard).ThenFunc(deps.Account.Profile))
	mux.Handle("POST /account/password", userStack.Extend(demoGuard).ThenFunc(deps.Account.Password))
	mux.Handle("GET /account/sessions", userStack.ThenFunc(deps.Account.Sessions))
	mux.Handle("DELETE /account/sessions/{id}", userStack.ThenFunc(deps.Account.RevokeSession))
	mux.Handle("POST /account/sessions/revoke-others", userStack.ThenFunc(deps.Account.RevokeOtherSessions))

	// USERS
	mux.Handle("GET /users", memberReadStack.ThenFunc(deps.User.Index))
	mux.Handle("POST /users", memberWriteStack.ThenFunc(deps.User.Store))
	mux.Handle("PUT /users/{id}", memberWriteStack.ThenFunc(deps.User.Update))
	mux.Handle("DELETE /users/{id}", memberWriteStack.ThenFunc(deps.User.Destroy))
	mux.Handle("POST /users/{id}/revoke-sessions", memberWriteStack.ThenFunc(deps.User.RevokeSessions))

	// APPLICATIONS
	mux.Handle("GET /applications", appReadStack.ThenFunc(deps.Application.Index))
	mux.Handle("GET /applications/{id}", appReadStack.ThenFunc(deps.Application.Show))
	mux.Handle("POST /applications/{id}/logs/tail", appReadStack.ThenFunc(deps.Application.TailLogs))
	mux.Handle("POST /applications/{id}/logs/tail/stop", appReadStack.ThenFunc(deps.Application.StopTailLogs))
	mux.Handle("POST /applications/{id}/logs/query", appReadStack.ThenFunc(deps.Application.QueryLogs))
	mux.Handle("POST /applications", appWriteStack.Extend(demoGuard).ThenFunc(deps.Application.Store))
	mux.Handle("PUT /applications/{id}", appWriteStack.ThenFunc(deps.Application.Update))
	mux.Handle("DELETE /applications/{id}", appWriteStack.Extend(demoGuard).ThenFunc(deps.Application.Destroy))

	// APPLICATION ACTIONS
	mux.Handle("POST /applications/{id}/deploy", appWriteStack.ThenFunc(deps.Application.Deploy))
	mux.Handle("POST /applications/{id}/rollback", appWriteStack.ThenFunc(deps.Application.Rollback))
	mux.Handle("POST /applications/{id}/start", appWriteStack.ThenFunc(deps.Application.Start))
	mux.Handle("POST /applications/{id}/stop", appWriteStack.ThenFunc(deps.Application.Stop))
	mux.Handle("POST /applications/{id}/restart", appWriteStack.ThenFunc(deps.Application.Restart))

	// DEPLOYMENTS
	mux.Handle("GET /applications/{id}/deployments", appReadStack.ThenFunc(deps.Deployment.Index))
	mux.Handle("GET /applications/{id}/deployments/{deployment_id}", appReadStack.ThenFunc(deps.Deployment.Show))
	mux.Handle("GET /applications/{id}/deployments/{deployment_id}/diff", appReadStack.ThenFunc(deps.Deployment.Diff))

	// AUDIT LOG
	mux.Handle("GET /audit-logs", userStack.ThenFunc(deps.AuditLog.Index))

	// SETTINGS (runtime-configurable knobs — webhook etc.)
	mux.Handle("GET /settings/webhook", userStack.ThenFunc(deps.Settings.GetWebhook))
	mux.Handle("PUT /settings/webhook", userStack.ThenFunc(deps.Settings.UpdateWebhook))
	mux.Handle("POST /settings/webhook/test", userStack.ThenFunc(deps.Settings.TestWebhook))

	// ENVIRONMENT VARIABLES
	mux.Handle("POST /applications/{id}/env", appWriteStack.ThenFunc(deps.Application.AddEnvVar))
	mux.Handle("PUT /applications/{id}/env/{key}", appWriteStack.ThenFunc(deps.Application.UpdateEnvVar))
	mux.Handle("DELETE /applications/{id}/env/{key}", appWriteStack.ThenFunc(deps.Application.DeleteEnvVar))

	// ALERTS
	if deps.Alert != nil {
		mux.Handle("GET /alerts/rules", alertReadStack.ThenFunc(deps.Alert.RulesIndex))
		mux.Handle("POST /alerts/rules", alertWriteStack.ThenFunc(deps.Alert.RulesStore))
		mux.Handle("GET /alerts/rules/{id}", alertReadStack.ThenFunc(deps.Alert.RulesShow))
		mux.Handle("PUT /alerts/rules/{id}", alertWriteStack.ThenFunc(deps.Alert.RulesUpdate))
		mux.Handle("DELETE /alerts/rules/{id}", alertWriteStack.ThenFunc(deps.Alert.RulesDestroy))

		mux.Handle("GET /alerts/active", alertReadStack.ThenFunc(deps.Alert.ActiveIndex))
		mux.Handle("GET /alerts/history", alertReadStack.ThenFunc(deps.Alert.HistoryIndex))
		mux.Handle("GET /alerts/history/{id}", alertReadStack.ThenFunc(deps.Alert.HistoryShow))

		mux.Handle("POST /alerts/{id}/ack", alertWriteStack.ThenFunc(deps.Alert.HistoryAck))
		mux.Handle("POST /alerts/{id}/silence", alertWriteStack.ThenFunc(deps.Alert.SilenceRule))
	}

	return globalMw.Apply(mux)
}
