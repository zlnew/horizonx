// Package workers
package workers

import (
	"context"
	"time"

	"horizonx-server/internal/domain"
	"horizonx-server/internal/logger"
)

type Manager struct {
	scheduler *Scheduler
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

func NewManager(scheduler *Scheduler, log logger.Logger, services *ManagerServices) *Manager {
	return &Manager{
		scheduler: scheduler,
		log:       log,

		services: services,
	}
}

func (m *Manager) Start(ctx context.Context) {
	m.log.Info("worker: manager started")

	m.scheduler.RunByDuration(ctx, 10*time.Second, &MetricsCollectWorker{
		job:    m.services.Job,
		server: m.services.Server,
		log:    m.log,
	})

	m.scheduler.RunDaily(ctx, DailySchedule{Hour: 2, Minute: 0}, &MetricsCleanupWorker{
		metrics: m.services.Metrics,
		server:  m.services.Server,
		log:     m.log,
	})

	m.scheduler.RunByDuration(ctx, 5*time.Minute, &ApplicationHealthCheckWorker{
		app: m.services.Application,
		job: m.services.Job,
		log: m.log,
	})
}
