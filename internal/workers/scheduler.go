package workers

import (
	"context"
	"time"

	"horizonx/internal/config"
	"horizonx/internal/logger"
)

type DailySchedule struct {
	Hour   int
	Minute int
}

// RunLocker serialises a worker run across server replicas.
type RunLocker interface {
	// TryLock reports whether this replica may run the named worker now.
	// acquired false means another replica holds it. release is non-nil
	// whenever acquired is true and must be called when the run finishes.
	TryLock(ctx context.Context, name string) (release func(), acquired bool, err error)
}

type Scheduler struct {
	cfg    *config.Config
	log    logger.Logger
	locker RunLocker
}

func NewScheduler(cfg *config.Config, log logger.Logger, locker RunLocker) *Scheduler {
	return &Scheduler{
		cfg:    cfg,
		log:    log,
		locker: locker,
	}
}

// runWorker acquires the cross-replica lock (when one is configured) and runs
// the worker, logging the outcome exactly once. A nil locker means no locking,
// which is correct for single-replica installs.
func (s *Scheduler) runWorker(ctx context.Context, worker Worker) {
	if s.locker != nil {
		release, acquired, err := s.locker.TryLock(ctx, worker.Name())
		if err != nil {
			// Fail closed: a lock we cannot take is never permission to run.
			s.log.Error("worker lock failed", "name", worker.Name(), "error", err)
			return
		}
		if !acquired {
			s.log.Debug("worker skipped", "name", worker.Name(), "reason", "another replica holds the lock")
			return
		}
		// Defer so the lock is released on every path, including a panic in
		// worker.Run.
		defer release()
	}

	start := time.Now()

	err := worker.Run(ctx)
	if err != nil {
		s.log.Error("worker failed", "name", worker.Name(), "error", err)
	}

	s.log.Debug("worker finished", "name", worker.Name(), "time", time.Since(start))
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
				s.runWorker(ctx, worker)
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
				s.runWorker(ctx, worker)
			}
		}
	}()
}
