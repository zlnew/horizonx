package workers

import (
	"context"
	"fmt"
	"time"

	"horizonx-server/internal/domain"
	"horizonx-server/internal/logger"
)

type MetricsCleanupWorker struct {
	metrics domain.MetricsService
	server  domain.ServerService
	log     logger.Logger
}

func NewMetricsCleanupWorker(metrics domain.MetricsService, server domain.ServerService, log logger.Logger) Worker {
	return &MetricsCleanupWorker{
		metrics: metrics,
		server:  server,
		log:     log,
	}
}

func (w *MetricsCleanupWorker) Name() string {
	return "metrics_cleanup"
}

func (w *MetricsCleanupWorker) Run(ctx context.Context) error {
	isOnline := true
	servers, err := w.server.List(ctx, domain.ServerListOptions{
		IsOnline: &isOnline,
	})
	if err != nil {
		return fmt.Errorf("failed to list online servers: %w", err)
	}

	now := time.Now().UTC()
	cutoffTime := time.Date(now.Year(), now.Month(), now.Day(), 2, 0, 0, 0, now.Location()).AddDate(0, 0, -7)

	for _, srv := range servers.Data {
		if err := w.metrics.Cleanup(ctx, srv.ID, cutoffTime); err != nil {
			w.log.Error("failed to cleanup server metrics", "server_id", srv.ID.String(), "error", err.Error())
		}
	}

	return nil
}
