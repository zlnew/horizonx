package main

import (
	"context"
	netHttp "net/http"
	"os/signal"
	"syscall"
	"time"

	"horizonx/internal/adapters/http"
	"horizonx/internal/adapters/http/request"
	"horizonx/internal/adapters/http/response"
	"horizonx/internal/adapters/http/validator"
	"horizonx/internal/adapters/postgres"
	"horizonx/internal/adapters/ws/agentws"
	"horizonx/internal/adapters/ws/userws"
	"horizonx/internal/adapters/ws/userws/subscribers"
	"horizonx/internal/application/account"
	"horizonx/internal/application/application"
	"horizonx/internal/application/auth"
	"horizonx/internal/application/deployment"
	"horizonx/internal/application/job"
	logSvc "horizonx/internal/application/log"
	"horizonx/internal/application/metrics"
	"horizonx/internal/application/role"
	"horizonx/internal/application/server"
	"horizonx/internal/application/user"
	"horizonx/internal/config"
	"horizonx/internal/event"
	"horizonx/internal/logger"
	"horizonx/internal/workers"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	log := logger.New(cfg)

	if cfg.JWTSecret == "" {
		panic("FATAL: JWT_SECRET is mandatory for Server!")
	}

	dbPool, err := postgres.InitDB(cfg.DatabaseURL)
	if err != nil {
		log.Error("failed to init DB", "error", err)
	} else {
		log.Info("postgres connected")
	}
	defer dbPool.Close()

	bus := event.New()

	// Repositories
	logRepo := postgres.NewLogRepository(dbPool)
	serverRepo := postgres.NewServerRepository(dbPool)
	roleRepo := postgres.NewRoleRepository(dbPool)
	userRepo := postgres.NewUserRepository(dbPool)
	jobRepo := postgres.NewJobRepository(dbPool)
	metricsRepo := postgres.NewMetricsRepository(dbPool)
	applicationRepo := postgres.NewApplicationRepository(dbPool)
	deploymentRepo := postgres.NewDeploymentRepository(dbPool)

	// Services
	logService := logSvc.NewService(logRepo, bus)
	serverService := server.NewService(serverRepo, bus)
	authService := auth.NewService(userRepo, cfg.JWTSecret, cfg.JWTExpiry)
	roleService := role.NewService(roleRepo)
	accountService := account.NewService(userRepo)
	userService := user.NewService(userRepo)
	jobService := job.NewService(jobRepo, logService, bus)
	metricsService := metrics.NewService(metricsRepo, bus, log)
	deploymentService := deployment.NewService(deploymentRepo, logService, bus)
	applicationService := application.NewService(applicationRepo, serverService, jobService, deploymentService, bus)

	// Event Listeners
	applicationListener := application.NewListener(applicationService, log)
	applicationListener.Register(bus)

	deploymentListener := deployment.NewListener(deploymentService, log)
	deploymentListener.Register(bus)

	// HTTP Handlers
	jsonDecoder := request.NewJSONDecoder()
	jsonWriter := response.NewJSONWriter(log)
	validator := validator.NewValidator()

	logHandler := http.NewLogHandler(logService, jsonDecoder, jsonWriter, validator)
	serverHandler := http.NewServerHandler(serverService, jsonDecoder, jsonWriter, validator)
	authHandler := http.NewAuthHandler(authService, cfg, jsonDecoder, jsonWriter, validator)
	accountHandler := http.NewAccountHandler(accountService, jsonDecoder, jsonWriter, validator)
	userHandler := http.NewUserHandler(userService, jsonDecoder, jsonWriter, validator)
	jobHandler := http.NewJobHandler(jobService, jsonDecoder, jsonWriter, validator)
	metricsHandler := http.NewMetricsHandler(metricsService, jsonDecoder, jsonWriter, validator)
	deploymentHandler := http.NewDeploymentHandler(deploymentService, jsonDecoder, jsonWriter, validator)
	applicationHandler := http.NewApplicationHandler(applicationService, jsonDecoder, jsonWriter, validator)

	// WebSocket Handlers
	wsUserhub := userws.NewHub(ctx, log)
	wsUserHandler := userws.NewHandler(wsUserhub, log, cfg.JWTSecret, cfg.AllowedOrigins)

	wsAgentRouter := agentws.NewRouter(ctx, log)
	wsAgentHandler := agentws.NewHandler(wsAgentRouter, log, serverService)

	go wsUserhub.Run()
	go wsAgentRouter.Run()

	// Register event subscribers
	subscribers.Register(bus, wsUserhub)

	router := http.NewRouter(cfg, &http.RouterDeps{
		WsUser:  wsUserHandler,
		WsAgent: wsAgentHandler,

		Auth:        authHandler,
		Account:     accountHandler,
		User:        userHandler,
		Server:      serverHandler,
		Log:         logHandler,
		Job:         jobHandler,
		Metrics:     metricsHandler,
		Application: applicationHandler,
		Deployment:  deploymentHandler,

		RoleService:   roleService,
		ServerService: serverService,
	})

	// Worker Manager
	wScheduler := workers.NewScheduler(cfg, log)
	wManager := workers.NewManager(log, wScheduler, &workers.ManagerServices{
		Job:         jobService,
		Server:      serverService,
		Metrics:     metricsService,
		Application: applicationService,
	})
	wManager.Start(ctx)

	// HTTP Server
	srv := http.NewServer(router, cfg.Address)

	errCh := make(chan error, 1)
	go func() {
		log.Info("http: starting server", "address", cfg.Address)
		errCh <- srv.ListenAndServe()
		close(errCh)
	}()

	select {
	case <-ctx.Done():
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
}
