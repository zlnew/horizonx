// Package app holds the runnable entrypoints shared by the standalone
// binaries (cmd/server, cmd/agent) and the unified `horizonx` CLI.
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sync/errgroup"

	"horizonx/internal/adapters/redis"
	"horizonx/internal/agent"
	"horizonx/internal/agent/docker"
	"horizonx/internal/agent/executor"
	"horizonx/internal/agent/logstream"
	"horizonx/internal/config"
	"horizonx/internal/domain"
	"horizonx/internal/logger"
	"horizonx/internal/metrics"
	"horizonx/internal/version"
)

// RunAgent starts the HorizonX agent and blocks until it stops.
func RunAgent() error {
	cfg := config.Load()
	appLog := logger.New(cfg)

	if cfg.AgentServerAPIToken == "" {
		return errors.New("HORIZONX_SERVER_API_TOKEN is missing in .env or system vars")
	}

	if cfg.AgentServerID.String() == "00000000-0000-0000-0000-000000000000" {
		return errors.New("HORIZONX_SERVER_ID is missing or invalid in .env")
	}

	appLog.Info("horizonx agent: starting...", "server_id", cfg.AgentServerID, "version", version.Version)

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

	// Apps work dir is ALWAYS the agent's current working directory (Maul,
	// 2026-08-04): no hardcoded /var/lib/horizonx/apps, no AppEnv condition.
	// In production the systemd unit sets WorkingDirectory=/var/lib/horizonx/apps,
	// so the executor lands on the same path via cwd — the unit is the single
	// source of truth for where apps live.
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getcwd: %w", err)
	}
	appsWorkDir := cwd

	registry := redis.NewRegistry(redisClient)
	httpClient := agent.NewHttpClient(cfg)
	collector := metrics.NewCollector(cfg, appLog, registry)
	exec := executor.NewExecutor(appsWorkDir, appLog, collector.Latest)
	worker := agent.NewJobWorker(cfg, appLog, *httpClient, *exec)
	conn := agent.NewAgent(cfg, appLog)

	// Container-log streams (A2): the manager needs the agent's send path
	// and the executor's apps workdir + compose-file resolution.
	logstreamMgr := logstream.NewManager(cfg.AgentServerID, appsWorkDir, docker.NewManager(), appLog, func(msg *domain.WsAgentMessage) error {
		return conn.SendMessage(msg)
	})
	conn.SetLogStream(logstreamMgr)

	if err := exec.Init(); err != nil {
		return fmt.Errorf("executor init: %w", err)
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

	return nil
}
