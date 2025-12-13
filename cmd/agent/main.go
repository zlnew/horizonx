package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"golang.org/x/sync/errgroup"

	"horizonx-server/internal/config"
	"horizonx-server/internal/core/agent"
	"horizonx-server/internal/logger"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("INFO: No .env file found, relying on system environment variables")
	}

	cfg := config.Load()
	appLog := logger.New(cfg)

	if cfg.AgentServerAPIToken == "" {
		log.Fatal("FATAL: HORIZONX_SERVER_API_TOKEN is missing in .env or system vars!")
	}

	appLog.Info("HorizonX Agent: starting spy mission...", "server_url", cfg.AgentTargetWsURL)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	a := agent.NewAgent(cfg, appLog)

	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return a.Run(gCtx)
	})

	// g.Go(func() error {
	// 	metricsSchedulerController(a, cfg, appLog, gCtx)
	// 	return nil
	// })

	if err := g.Wait(); err != nil && err != context.Canceled && !agent.IsFatalError(err) {
		appLog.Error("agent failed unexpectedly", "error", err)
	} else if agent.IsFatalError(err) {
		appLog.Error("agent failed fatally, exiting", "error", err)
	}

	appLog.Info("agent stopped gracefully.")
}

// func metricsSchedulerController(a *agent.Agent, cfg *config.Config, appLog logger.Logger, shutdownCtx context.Context) {
// 	metricsSampler := metrics.NewSampler(appLog)
// 	metricsSink := func(m domain.Metrics) {
// 		a.SendMetrics(m)
// 	}
//
// 	for {
// 		select {
// 		case sessionCtx := <-a.GetSessionContextChannel():
// 			appLog.Info("starting metrics scheduler for new session")
// 			metricsScheduler := metrics.NewScheduler(cfg.AgentMetricsInterval, appLog, metricsSampler.Collect, metricsSink)
//
// 			metricsScheduler.Start(sessionCtx)
//
// 			if shutdownCtx.Err() != nil {
// 				appLog.Info("metrics scheduler controller shutting down due to main context cancellation.")
// 				return
// 			}
//
// 			appLog.Info("metrics scheduler stopped as session context was cancelled, waiting for new session...")
//
// 		case <-shutdownCtx.Done():
// 			appLog.Info("metrics scheduler controller received main shutdown signal.")
// 			return
//
// 		case <-time.After(10 * time.Second):
// 			appLog.Debug("metrics scheduler controller is alive, waiting for session context...")
// 		}
// 	}
// }
