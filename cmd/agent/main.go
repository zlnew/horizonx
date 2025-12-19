package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"golang.org/x/sync/errgroup"

	"horizonx-server/internal/agent"
	"horizonx-server/internal/config"
	"horizonx-server/internal/logger"
	"horizonx-server/internal/metrics"
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

	if cfg.AgentServerID.String() == "00000000-0000-0000-0000-000000000000" {
		log.Fatal("FATAL: HORIZONX_SERVER_ID is missing or invalid in .env!")
	}

	appLog.Info("HorizonX Agent: starting...", "server_id", cfg.AgentServerID)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	a := agent.NewAgent(cfg, appLog)
	sampler := metrics.NewSampler(appLog)
	sampler.SetServerID(cfg.AgentServerID)

	reporter := agent.NewMetricsReporter(
		cfg.AgentTargetAPIURL,
		cfg.AgentServerID.String()+"."+cfg.AgentServerAPIToken,
		appLog,
	)

	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return a.Run(gCtx)
	})

	g.Go(func() error {
		return runMetricsCollector(gCtx, cfg, sampler, reporter, appLog)
	})

	if err := g.Wait(); err != nil && err != context.Canceled && !agent.IsFatalError(err) {
		appLog.Error("agent failed unexpectedly", "error", err)
	} else if agent.IsFatalError(err) {
		appLog.Error("agent failed fatally, exiting", "error", err)
	}

	appLog.Info("agent stopped gracefully.")
}

func runMetricsCollector(
	ctx context.Context,
	cfg *config.Config,
	sampler *metrics.Sampler,
	reporter *agent.MetricsReporter,
	log logger.Logger,
) error {
	collectionTicker := time.NewTicker(cfg.AgentMetricsCollectInterval)
	flushTicker := time.NewTicker(cfg.AgentMetricsFlushInterval)
	defer collectionTicker.Stop()
	defer flushTicker.Stop()

	log.Info("metrics collector started",
		"collection_interval", cfg.AgentMetricsCollectInterval,
		"flush_interval", cfg.AgentMetricsFlushInterval,
	)

	for {
		select {
		case <-ctx.Done():
			log.Info("flushing remaining metrics before shutdown...")
			flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			reporter.Flush(flushCtx)
			cancel()
			return ctx.Err()

		case <-collectionTicker.C:
			metrics := sampler.Collect(ctx)
			metrics.RecordedAt = time.Now().UTC()

			reporter.Add(metrics)

			log.Debug("metrics collected", "buffer_size", reporter.BufferSize())

			if reporter.ShouldFlush() {
				log.Debug("buffer full, forcing flush")
				if err := reporter.Flush(ctx); err != nil {
					log.Error("failed to flush metrics", "error", err)
				}
			}

		case <-flushTicker.C:
			if err := reporter.Flush(ctx); err != nil {
				log.Error("failed to flush metrics", "error", err)
			}
		}
	}
}
