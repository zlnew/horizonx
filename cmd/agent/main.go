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
	"horizonx/internal/agent/executor"
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

	registry := redis.NewRegistry(redisClient)
	httpClient := agent.NewHttpClient(cfg)
	collector := metrics.NewCollector(cfg, appLog, registry)
	executor := executor.NewExecutor(appLog, collector.Latest)
	worker := agent.NewJobWorker(cfg, appLog, *httpClient, *executor)
	conn := agent.NewAgent(cfg, appLog)

	if err := executor.Init(); err != nil {
		appLog.Error("failed to init executor", "error", err)
		log.Fatal(err)
	}

	g, gctx := errgroup.WithContext(runtimeCtx)

	g.Go(func() error {
		return collector.Start(gctx)
	})

	g.Go(func() error {
		return worker.Start(gctx)
	})

	g.Go(func() error {
		return conn.Start(gctx)
	})

	if err := g.Wait(); err != nil {
		appLog.Error("agent stopped with error", "error", err)
	} else {
		appLog.Info("agent stopped gracefully.")
	}
}
