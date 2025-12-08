package main

import (
	"context"
	"errors"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"horizonx-server/api/rest"
	"horizonx-server/internal/config"
	"horizonx-server/internal/core"
	"horizonx-server/internal/core/metrics"
	"horizonx-server/internal/logger"
	"horizonx-server/internal/storage/snapshot"
	"horizonx-server/internal/storage/sqlite"
	"horizonx-server/internal/transport/websocket"
	"horizonx-server/pkg/types"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	log := logger.New(cfg)

	ms := snapshot.NewMetricsStore()
	hub := websocket.NewHub(log)
	sampler := metrics.NewSampler(log)

	sched := core.NewScheduler(cfg.Interval, log, sampler.Collect, func(m types.Metrics) {
		ms.Set(m)
		hub.Emit("metrics", "metrics.updated", m)
	})
	go sched.Start(ctx)
	go hub.Run()

	db, err := sqlite.NewSqliteDB(cfg.DBPath, log)
	if err != nil {
		log.Error("sqlite", "connect", err)
		return
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Error("sqlite", "close", err)
		}
	}()

	router := rest.NewRouter(cfg, ms, hub, db, log)
	srv := &http.Server{
		Addr:         cfg.Address,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

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
