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

	appLog.Info("HorizonX Agent: starting spy mission...")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	a := agent.NewAgent(cfg, appLog)

	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return a.Run(gCtx)
	})

	if err := g.Wait(); err != nil && err != context.Canceled && !agent.IsFatalError(err) {
		appLog.Error("agent failed unexpectedly", "error", err)
	} else if agent.IsFatalError(err) {
		appLog.Error("agent failed fatally, exiting", "error", err)
	}

	appLog.Info("agent stopped gracefully.")
}
