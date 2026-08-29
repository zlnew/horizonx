// Package app holds the runnable entrypoints shared by the standalone
// binaries (cmd/server, cmd/agent) and the unified `horizonx` CLI.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	netHttp "net/http"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"horizonx/internal/adapters/http"
	httpmetrics "horizonx/internal/adapters/http/metrics"
	"horizonx/internal/adapters/http/request"
	"horizonx/internal/adapters/http/response"
	"horizonx/internal/adapters/http/validator"
	"horizonx/internal/adapters/postgres"
	"horizonx/internal/adapters/redis"
	"horizonx/internal/adapters/webhook"
	"horizonx/internal/adapters/ws/agentws"
	"horizonx/internal/adapters/ws/userws"
	"horizonx/internal/adapters/ws/userws/subscribers"
	"horizonx/internal/alerting"
	"horizonx/internal/application/account"
	"horizonx/internal/application/alert"
	"horizonx/internal/application/application"
	"horizonx/internal/application/auditlog"
	"horizonx/internal/application/auth"
	"horizonx/internal/application/deployment"
	"horizonx/internal/application/job"
	logSvc "horizonx/internal/application/log"
	"horizonx/internal/application/metrics"
	"horizonx/internal/application/role"
	"horizonx/internal/application/server"
	"horizonx/internal/application/user"
	"horizonx/internal/config"
	"horizonx/internal/domain"
	"horizonx/internal/event"
	"horizonx/internal/logger"
	"horizonx/internal/security"
	"horizonx/internal/workers"
)

// RunServer starts the HorizonX control-plane server and blocks until it stops.
func RunServer() error {
	ctx := context.Background()
	runtimeCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	log := logger.New(cfg)

	// P1-12: JWT_SECRET is the linchpin of auth + env-var encryption (P1-11).
	// Refuse to boot in production with an empty or known-dev default secret —
	// a predictable secret means forged tokens and decryptable env vars.
	devDefaults := map[string]bool{
		"":                    true,
		"changeme-dev-secret": true,
		"secret":              true,
	}
	if cfg.JWTSecret == "" || (cfg.AppEnv == "production" && devDefaults[cfg.JWTSecret]) {
		return errors.New("JWT_SECRET must be set to a strong random value (production refuses dev defaults)")
	}

	// Auto-migrate before serving (Laravel-style). Never serve on a stale
	// schema; golang-migrate's advisory lock keeps concurrent boots safe.
	if cfg.AutoMigrate {
		log.Info("migrating database schema…")
		ver, dirty, err := AutoMigrate(cfg.DatabaseURL)
		if err != nil {
			return fmt.Errorf("auto-migrate: %w", err)
		}
		if dirty {
			log.Warn("schema is dirty at version %d — check migrations", ver)
		} else {
			log.Info("schema up to date", "version", ver)
		}
	} else {
		log.Info("auto-migrate disabled (AUTO_MIGRATE=false)")
	}

	// Fail fast on DB/Redis init: continuing with a nil pool means every
	// handler 500s and `defer dbPool.Close()` panics on nil. A control
	// plane that can't reach its stores is useless — better to exit loudly.
	dbPool, err := postgres.Init(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("failed to init postgres: %w", err)
	}
	log.Info("postgres connected")
	defer dbPool.Close()

	redisClient, err := redis.Init(ctx, &redis.ClientOptions{
		Address:  cfg.RedisAddress,
		Username: cfg.RedisUsername,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	if err != nil {
		return fmt.Errorf("failed to init redis: %w", err)
	}
	log.Info("redis connected")
	defer redisClient.Close()

	bus := event.New()
	redisRegistry := redis.NewRegistry(redisClient)

	// Repositories
	logRepo := postgres.NewLogRepository(dbPool)
	serverRepo := postgres.NewServerRepository(dbPool)
	roleRepo := postgres.NewRoleRepository(dbPool)
	userRepo := postgres.NewUserRepository(dbPool)
	jobRepo := postgres.NewJobRepository(dbPool)
	metricsRepo := postgres.NewMetricsRepository(dbPool)
	// P1-11: env var values are encrypted at rest with a key derived from
	// JWT_SECRET (AES-256-GCM). No second secret to manage.
	applicationRepo := postgres.NewApplicationRepository(dbPool, security.KeyFromSecret(cfg.JWTSecret))
	deploymentRepo := postgres.NewDeploymentRepository(dbPool)
	auditLogRepo := postgres.NewAuditLogRepository(dbPool)
	settingsRepo := postgres.NewSettingsRepository(dbPool)
	alertRuleRepo := postgres.NewAlertRuleRepository(dbPool)
	alertHistoryRepo := postgres.NewAlertHistoryRepository(dbPool)

	// Services
	logService := logSvc.NewService(logRepo, bus)
	serverService := server.NewService(serverRepo, bus)
	sessionStore := redis.NewSessionStore(redisClient)
	authService := auth.NewService(userRepo, sessionStore, cfg.JWTSecret, cfg.JWTExpiry)
	roleService := role.NewService(roleRepo)
	accountService := account.NewService(userRepo, sessionStore)
	userService := user.NewService(userRepo)
	jobService := job.NewService(jobRepo, logService, bus)
	metricsService := metrics.NewService(metricsRepo, redisRegistry, bus, log)
	// The agent WS router must exist before the application service (A2
	// logs send commands through it) and the server handler (A1 ping proof).
	wsAgentRouter := agentws.NewRouter(runtimeCtx, log)

	deploymentService := deployment.NewService(deploymentRepo, logService, bus)
	applicationService := application.NewService(applicationRepo, serverService, jobService, deploymentService, bus, wsAgentRouter)
	auditLogService := auditlog.NewService(auditLogRepo)
	alertService := alert.NewService(alertRuleRepo, alertHistoryRepo)

	// Rebuild role→permission grants on EVERY boot, not only on first-boot
	// seeding. SyncPermissions is idempotent (upserts roles/permissions and
	// rebuilds the role_has_permissions pivot in one transaction), so existing
	// DBs pick up newly added permissions (e.g. alert_read/alert_write)
	// without a fresh install or a dedicated migration.
	if err := roleService.SyncPermissions(ctx); err != nil {
		return fmt.Errorf("sync permissions: %w", err)
	}

	// Auto-seed the admin user (Laravel-style seeding, like auto-migrate).
	// The .env (ADMIN_EMAIL / ADMIN_PASSWORD) seeds the admin on FIRST boot.
	// If the user already exists we do NOT touch it — the password belongs
	// to the account, not the .env, and re-running install (or an operator
	// editing .env) must never reset an existing admin. `install server`
	// prints the credentials only on first install for this reason.
	// Opt out entirely with AUTO_SEED=false.
	if cfg.AutoSeed {
		if cfg.AdminPass == "" {
			return errors.New("auto-seed: ADMIN_PASSWORD is empty (set it in the instance .env, or run `horizonx install server`)")
		}
		if _, err := userRepo.GetByEmail(ctx, cfg.AdminEmail); errors.Is(err, domain.ErrUserNotFound) {
			log.Info("auto-seeding admin user", "email", cfg.AdminEmail)
			req := domain.UserSaveRequest{
				Name:     "Admin",
				Email:    cfg.AdminEmail,
				Password: cfg.AdminPass,
				RoleID:   1,
			}
			if err := userService.Create(ctx, req); err != nil {
				return fmt.Errorf("auto-seed: create admin: %w", err)
			}
			log.Info("admin user seeded", "email", cfg.AdminEmail)
		} else if err != nil {
			return fmt.Errorf("auto-seed: check admin: %w", err)
		}
	} else {
		log.Info("auto-seed disabled (AUTO_SEED=false)")
	}

	// Event Listeners
	applicationListener := application.NewListener(applicationService, log)
	applicationListener.Register(bus)

	deploymentListener := deployment.NewListener(deploymentService, log)
	deploymentListener.Register(bus)

	// P2-14: Prometheus registry (request counters + job queue gauges).
	metricsRegistry := httpmetrics.NewRegistry(jobRepo, serverRepo, log)

	// P2-15: deploy-event webhook. Settings come from the settings repo
	// (changeable from the dashboard); WEBHOOK_URL env remains a fallback for
	// existing installs until the first dashboard save, so a pre-0.3.13
	// install keeps working without reconfiguration.
	notifier := webhook.New(func() domain.WebhookSettings {
		raw, err := settingsRepo.Get(runtimeCtx, domain.SettingWebhook)
		if err == nil {
			var ws domain.WebhookSettings
			if json.Unmarshal(raw, &ws) == nil {
				return ws
			}
		}
		return domain.WebhookSettings{Enabled: cfg.WebhookURL != "", URL: cfg.WebhookURL}
	}, applicationService, log)
	bus.Subscribe("deployment_status_changed", notifier.Handle)
	bus.Subscribe("alert_fired", notifier.Handle)
	bus.Subscribe("alert_resolved", notifier.Handle)

	// P3-21: alert evaluator. Subscribes to the metrics / status / health
	// bus topics, evaluates enabled rules live (per event), persists fires
	// and resolutions, and publishes alert_fired / alert_resolved which the
	// webhook notifier (above) relays.
	alerting.New(bus, log).
		WithProvider(alertRuleRepo).
		WithHistory(alertHistoryRepo).
		Start()

	// P3-19: audit log — record deploy/app/server events.
	auditSubscriber := auditlog.NewSubscriber(auditLogService)
	auditSubscriber.Register(bus)

	// WebSocket Handlers — the agent router must exist before the server
	// handler (A1's ping proof sends commands through it).
	wsUserhub := userws.NewHub(runtimeCtx, log)
	wsUserHandler := userws.NewHandler(wsUserhub, log, cfg.JWTSecret, cfg.AllowedOrigins)

	wsAgentHandler := agentws.NewHandler(wsAgentRouter, log, serverService)
	wsAgentHandler.SetPublisher(bus.Publish)

	// Track active log-tail stream IDs so channel-empty can stop the right
	// stream (A2). Defined before the handler uses it.
	appLogStreams := make(map[int64]string)
	var appLogMu sync.Mutex
	trackedLogs := &trackedContainerLogs{
		inner:   applicationService,
		streams: appLogStreams,
		mu:      &appLogMu,
	}

	// HTTP Handlers
	jsonDecoder := request.NewJSONDecoder()
	jsonWriter := response.NewJSONWriter(log)
	validator := validator.NewValidator()

	logHandler := http.NewLogHandler(logService, jsonDecoder, jsonWriter, validator)
	serverHandler := http.NewServerHandler(serverService, wsAgentRouter, jsonDecoder, jsonWriter, validator)
	authHandler := http.NewAuthHandler(authService, cfg, jsonDecoder, jsonWriter, validator)
	accountHandler := http.NewAccountHandler(accountService, jsonDecoder, jsonWriter, validator)
	userHandler := http.NewUserHandler(userService, authService, jsonDecoder, jsonWriter, validator)
	jobHandler := http.NewJobHandler(jobService, jsonDecoder, jsonWriter, validator)
	metricsHandler := http.NewMetricsHandler(metricsService, jsonDecoder, jsonWriter, validator)
	deploymentHandler := http.NewDeploymentHandler(deploymentService, jsonDecoder, jsonWriter, validator)
	applicationHandler := http.NewApplicationHandler(applicationService, trackedLogs, jsonDecoder, jsonWriter, validator)
	auditLogHandler := http.NewAuditLogHandler(auditLogService, jsonDecoder, jsonWriter, validator)
	settingsHandler := http.NewSettingsHandler(settingsRepo, notifier, jsonDecoder, jsonWriter, validator)
	alertHandler := http.NewAlertHandler(alertService, jsonDecoder, jsonWriter, validator)

	go wsUserhub.Run()
	go wsAgentRouter.Run()

	// Stop container-log tails when their app_logs channel empties (browser
	// tab closed). The stream ID was recorded by trackedContainerLogs when
	// the tail started, so the stop command targets the right stream.
	wsUserhub.SetChannelEmptyHandler(func(channel string) {
		const prefix = "app_logs:"
		if !strings.HasPrefix(channel, prefix) {
			return
		}
		appID, err := strconv.ParseInt(strings.TrimPrefix(channel, prefix), 10, 64)
		if err != nil {
			return
		}
		appLogMu.Lock()
		streamID := appLogStreams[appID]
		delete(appLogStreams, appID)
		appLogMu.Unlock()
		if streamID == "" {
			return
		}
		ctx, cancel := context.WithTimeout(runtimeCtx, 5*time.Second)
		defer cancel()
		if err := applicationService.StopTailLogs(ctx, appID, streamID); err != nil {
			log.Warn("failed to stop log tail on channel empty", "app_id", appID, "error", err)
		}
	})

	// Register event subscribers
	subscribers.Register(bus, wsUserhub)

	router := http.NewRouter(cfg, &http.RouterDeps{
		WsUser:       wsUserHandler,
		WsAgent:      wsAgentHandler,
		WsAgentRoute: wsAgentRouter,

		Auth:        authHandler,
		Account:     accountHandler,
		User:        userHandler,
		Server:      serverHandler,
		Log:         logHandler,
		Job:         jobHandler,
		Metrics:     metricsHandler,
		Application: applicationHandler,
		Deployment:  deploymentHandler,
		AuditLog:    auditLogHandler,
		Settings:    settingsHandler,
		Alert:       alertHandler,

		SessionStore: sessionStore,

		RoleService:   roleService,
		ServerService: serverService,

		MetricsRegistry: metricsRegistry,
		Logger:          log,
	})

	// Worker Manager
	wScheduler := workers.NewScheduler(cfg, log)
	wManager := workers.NewManager(log, wScheduler, &workers.ManagerServices{
		Job:         jobService,
		Server:      serverService,
		Metrics:     metricsService,
		Application: applicationService,
	})
	wManager.Start(runtimeCtx)

	// HTTP Server
	srv := http.NewServer(router, cfg.Address)

	errCh := make(chan error, 1)
	go func() {
		log.Info("http: starting server", "address", cfg.Address)
		errCh <- srv.ListenAndServe()
		close(errCh)
	}()

	select {
	case <-runtimeCtx.Done():
		wsUserhub.Stop()
		wsAgentRouter.Stop()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Error("http: server shutdown error", "error", err)
		}

	case err := <-errCh:
		if err != nil && err != netHttp.ErrServerClosed {
			log.Error("http: server error", "error", err)
		}
	}

	log.Info("server stopped")
	return nil
}
