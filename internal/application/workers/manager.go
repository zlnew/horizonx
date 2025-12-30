// Package workers
package workers

import (
	"context"
	"time"

	"horizonx-server/internal/config"
	"horizonx-server/internal/domain"
	"horizonx-server/internal/logger"
)

type Manager struct {
	scheduler *Scheduler
	cfg       *config.Config
	log       logger.Logger

	services *ManagerServices
}

type ManagerServices struct {
	Job         domain.JobService
	Server      domain.ServerService
	Metrics     domain.MetricsService
	Application domain.ApplicationService
}

type Worker interface {
	Name() string
	Run(ctx context.Context) error
}

func NewManager(scheduler *Scheduler, cfg *config.Config, log logger.Logger, services *ManagerServices) *Manager {
	return &Manager{
		scheduler: scheduler,
		cfg:       cfg,
		log:       log,

		services: services,
	}
}

func (m *Manager) Start(ctx context.Context) {
	m.log.Info("worker: manager started")

	m.scheduler.RunByDuration(ctx, m.cfg.MetricsCollectInterval, &MetricsCollectWorker{
		job:    m.services.Job,
		server: m.services.Server,
		log:    m.log,
	})

	m.scheduler.RunDaily(ctx, DailySchedule{Hour: 2, Minute: 0}, &MetricsCleanupWorker{
		metrics: m.services.Metrics,
		server:  m.services.Server,
		log:     m.log,
	})

	m.scheduler.RunByDuration(ctx, 10*time.Minute, &ApplicationHealthCheckWorker{
		svc: m.services.Application,
		log: m.log,
	})
}
