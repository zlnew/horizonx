package main

import (
	"context"
	netHttp "net/http"
	"os/signal"
	"syscall"
	"time"

	"horizonx-server/internal/adapters/http"
	"horizonx-server/internal/adapters/postgres"
	"horizonx-server/internal/adapters/ws/agentws"
	"horizonx-server/internal/adapters/ws/userws"
	"horizonx-server/internal/adapters/ws/userws/subscribers"
	"horizonx-server/internal/application/application"
	"horizonx-server/internal/application/auth"
	"horizonx-server/internal/application/deployment"
	"horizonx-server/internal/application/job"
	logService "horizonx-server/internal/application/log"
	"horizonx-server/internal/application/metrics"
	"horizonx-server/internal/application/server"
	"horizonx-server/internal/application/user"
	"horizonx-server/internal/application/workers"
	"horizonx-server/internal/config"
	"horizonx-server/internal/event"
	"horizonx-server/internal/logger"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	log := logger.New(cfg)

	if cfg.JWTSecret == "" {
		panic("FATAL: JWT_SECRET is mandatory for Server!")
	}

	dbPool, err := postgres.InitDB(cfg.DatabaseURL, log)
	if err != nil {
		log.Error("failed to init DB", "error", err)
	}
	defer dbPool.Close()

	bus := event.New()

	// Repositories
	logRepo := postgres.NewLogRepository(dbPool)
	serverRepo := postgres.NewServerRepository(dbPool)
	userRepo := postgres.NewUserRepository(dbPool)
	jobRepo := postgres.NewJobRepository(dbPool)
	metricsRepo := postgres.NewMetricsRepository(dbPool)
	applicationRepo := postgres.NewApplicationRepository(dbPool)
	deploymentRepo := postgres.NewDeploymentRepository(dbPool)

	// Services
	logService := logService.NewService(logRepo, bus)
	serverService := server.NewService(serverRepo, bus)
	authService := auth.NewService(userRepo, cfg.JWTSecret, cfg.JWTExpiry)
	userService := user.NewService(userRepo)
	jobService := job.NewService(jobRepo, logService, bus)
	metricsService := metrics.NewService(cfg, metricsRepo, bus, log)
	deploymentService := deployment.NewService(deploymentRepo, logService, bus)
	applicationService := application.NewService(applicationRepo, serverService, jobService, deploymentService, bus)

	// Event Listeners
	applicationListener := application.NewListener(applicationService, log)
	applicationListener.Register(bus)

	deploymentListener := deployment.NewListener(deploymentService, log)
	deploymentListener.Register(bus)

	// HTTP Handlers
	logHandler := http.NewLogHandler(logService)
	serverHandler := http.NewServerHandler(serverService)
	authHandler := http.NewAuthHandler(authService, cfg)
	userHandler := http.NewUserHandler(userService)
	jobHandler := http.NewJobHandler(jobService)
	metricsHandler := http.NewMetricsHandler(metricsService)
	deploymentHandler := http.NewDeploymentHandler(deploymentService)
	applicationHandler := http.NewApplicationHandler(applicationService)

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
		User:        userHandler,
		Server:      serverHandler,
		Log:         logHandler,
		Job:         jobHandler,
		Metrics:     metricsHandler,
		Application: applicationHandler,
		Deployment:  deploymentHandler,

		ServerService: serverService,
	})

	// Worker Manager
	wSheduler := workers.NewScheduler(log)
	wManager := workers.NewManager(wSheduler, cfg, log, &workers.ManagerServices{
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
