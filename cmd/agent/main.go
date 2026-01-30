package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"golang.org/x/sync/errgroup"

	"horizonx/internal/adapters/redis"
	"horizonx/internal/agent"
	"horizonx/internal/config"
	"horizonx/internal/logger"
	"horizonx/internal/metrics"
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

	appLog.Info("horizonx agent: starting...", "server_id", cfg.AgentServerID)

	ctx := context.Background()
	runtimeCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	redisClient, err := redis.Init(ctx, &redis.ClientOptions{
		Address:  cfg.RedisAddress,
		Username: cfg.RedisUsername,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	if err != nil {
		appLog.Error("failed to init redis", "error", err)
	} else {
		appLog.Info("redis connected")
	}
	defer redisClient.Close()

	// Initialize components
	ws := agent.NewAgent(cfg, appLog)
	mRegistry := redis.NewRegistry(redisClient)
	mCollector := metrics.NewCollector(cfg, appLog, mRegistry)

	// Initialize job worker
	jWorker := agent.NewJobWorker(cfg, appLog, mCollector.Latest)
	if err := jWorker.Initialize(); err != nil {
		appLog.Error("failed to Initialize job worker", "error", err)
		log.Fatal(err)
	}

	g, gCtx := errgroup.WithContext(runtimeCtx)

	// WebSocket connection
	g.Go(func() error {
		return ws.Run(gCtx)
	})

	// Metrics collector
	g.Go(func() error {
		return mCollector.Start(gCtx)
	})

	// Job worker
	g.Go(func() error {
		return jWorker.Start(gCtx)
	})

	if err := g.Wait(); err != nil && err != context.Canceled && !agent.IsFatalError(err) {
		appLog.Error("agent failed unexpectedly", "error", err)
	} else if agent.IsFatalError(err) {
		appLog.Error("agent failed fatally, exiting", "error", err)
	}

	appLog.Info("agent stopped gracefully.")
}
