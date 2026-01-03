package workers

import (
	"context"
	"time"

	"horizonx-server/internal/config"
	"horizonx-server/internal/logger"
)

type DailySchedule struct {
	Hour   int
	Minute int
}

type Scheduler struct {
	cfg *config.Config
	log logger.Logger
}

func NewScheduler(cfg *config.Config, log logger.Logger) *Scheduler {
	return &Scheduler{
		cfg: cfg,
		log: log,
	}
}

func (s *Scheduler) RunByDuration(ctx context.Context, dur time.Duration, worker Worker) {
	go func() {
		ticker := time.NewTicker(dur)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				start := time.Now()

				err := worker.Run(ctx)
				if err != nil {
					s.log.Error("worker failed", "name", worker.Name(), "error", err)
				}

				s.log.Debug("worker finished", "name", worker.Name(), "time", time.Since(start))
			}
		}
	}()
}

func (s *Scheduler) RunDaily(ctx context.Context, schedule DailySchedule, worker Worker) {
	go func() {
		for {
			now := time.Now().In(s.cfg.TimeZone)

			next := time.Date(
				now.Year(),
				now.Month(),
				now.Day(),
				schedule.Hour,
				schedule.Minute,
				0,
				0,
				s.cfg.TimeZone,
			)
			if !next.After(now) {
				next = next.AddDate(0, 0, 1)
			}

			timer := time.NewTimer(time.Until(next))

			select {
			case <-ctx.Done():
				timer.Stop()
				s.log.Debug("daily worker canceled", "name", worker.Name())
				return
			case <-timer.C:
				start := time.Now()

				err := worker.Run(ctx)
				if err != nil {
					s.log.Error("worker failed", "name", worker.Name(), "error", err)
				}

				s.log.Debug("worker finished", "name", worker.Name(), "time", time.Since(start))
			}
		}
	}()
}
