package main

import (
	"context"
	"errors"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"horizonx-server/internal/config"
	"horizonx-server/internal/core/auth"
	"horizonx-server/internal/core/metrics"
	"horizonx-server/internal/core/server"
	"horizonx-server/internal/core/user"
	"horizonx-server/internal/logger"
	"horizonx-server/internal/storage/postgres"
	"horizonx-server/internal/storage/snapshot"
	"horizonx-server/internal/transport/rest"
	"horizonx-server/internal/transport/websocket"
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
		log.Error("Failed to init DB", "error", err)
	}
	defer dbPool.Close()

	metricsRepo := postgres.NewMetricsRepository(dbPool)
	metricsStore := snapshot.NewMetricsStore()

	serverRepo := postgres.NewServerRepository(dbPool)
	userRepo := postgres.NewUserRepository(dbPool)

	serverService := server.NewService(serverRepo)
	authService := auth.NewService(userRepo, cfg.JWTSecret, cfg.JWTExpiry)
	userService := user.NewService(userRepo)

	hub := websocket.NewHub(log, serverService)
	go hub.Run()

	metrics.NewService(metricsRepo, metricsStore, hub, log)

	wsHandler := websocket.NewHandler(hub, cfg, log, serverService)
	serverHandler := rest.NewServerHandler(serverService)
	authHandler := rest.NewAuthHandler(authService, cfg)
	userHandler := rest.NewUserHandler(userService)

	router := rest.NewRouter(cfg, &rest.RouterDeps{
		WS:     wsHandler,
		Server: serverHandler,
		Auth:   authHandler,
		User:   userHandler,

		ServerRepo: serverRepo,
	})

	srv := rest.NewServer(router, cfg.Address)

	errCh := make(chan error, 1)
	go func() {
		log.Info("starting http server", "address", cfg.Address)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Error("http server shutdown error", "error", err)
		}

	case err := <-errCh:
		log.Error("http server error", "error", err)
	}

	log.Info("server stopped")
}
