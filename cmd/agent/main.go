package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"golang.org/x/sync/errgroup"

	"horizonx-server/internal/agent"
	"horizonx-server/internal/config"
	"horizonx-server/internal/core/metrics"
	"horizonx-server/internal/domain"
	"horizonx-server/internal/logger"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("INFO: No .env file found, relying on system environment variables")
	}

	cfg := config.Load()
	appLog := logger.New(cfg)

	agentToken := os.Getenv("HORIZONX_AGENT_TOKEN")
	if agentToken == "" {
		log.Fatal("FATAL: HORIZONX_AGENT_TOKEN is missing in .env or system vars!")
	}

	serverURL := os.Getenv("HORIZONX_SERVER_URL")
	if serverURL == "" {
		serverURL = "ws://localhost:3000/ws"
	}

	appLog.Info("HorizonX Agent: starting spy mission...", "server_url", serverURL)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	h := agent.NewHub(ctx, appLog)
	go h.Run()
	a := agent.NewAgent(h, appLog, serverURL, agentToken)
	h.SetAgent(a)

	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return a.Run(gCtx)
	})

	g.Go(func() error {
		metricsSchedulerController(a, cfg, appLog, gCtx)
		return nil
	})

	if err := g.Wait(); err != nil && err != context.Canceled && !agent.IsFatalError(err) {
		appLog.Error("agent failed unexpectedly", "error", err)
	} else if agent.IsFatalError(err) {
		appLog.Error("agent failed fatally, exiting", "error", err)
	}

	h.Stop()

	appLog.Info("agent stopped gracefully.")
}

func metricsSchedulerController(a *agent.Agent, cfg *config.Config, appLog logger.Logger, shutdownCtx context.Context) {
	metricsSampler := metrics.NewSampler(appLog)
	metricsSink := func(m domain.Metrics) {
		a.SendMetrics(m)
	}

	for {
		select {
		case sessionCtx := <-a.GetSessionContextChannel():
			appLog.Info("starting metrics scheduler for new session")
			metricsScheduler := metrics.NewScheduler(cfg.Interval, appLog, metricsSampler.Collect, metricsSink)

			metricsScheduler.Start(sessionCtx)

			if shutdownCtx.Err() != nil {
				appLog.Info("metrics scheduler controller shutting down due to main context cancellation.")
				return
			}

			appLog.Info("metrics scheduler stopped as session context was cancelled, waiting for new session...")

		case <-shutdownCtx.Done():
			appLog.Info("metrics scheduler controller received main shutdown signal.")
			return

		case <-time.After(10 * time.Second):
			appLog.Debug("metrics scheduler controller is alive, waiting for session context...")
		}
	}
}
