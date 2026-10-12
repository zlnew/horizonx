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

// WorkerFailureReporter counts worker runs that failed after exhausting
// retries. Nil-safe: a nil reporter means no metrics.
type WorkerFailureReporter interface {
	WorkerRunFailed(worker string)
}

// retryPolicy bounds how many times a failing worker run is retried.
type retryPolicy struct {
	attempts int           // total runs, including the first
	base     time.Duration // wait before the first retry
	max      time.Duration // cap on the wait
}

type Scheduler struct {
	cfg      *config.Config
	log      logger.Logger
	locker   RunLocker
	reporter WorkerFailureReporter
	retry    retryPolicy
}

func NewScheduler(cfg *config.Config, log logger.Logger, locker RunLocker, reporter WorkerFailureReporter) *Scheduler {
	return &Scheduler{
		cfg:      cfg,
		log:      log,
		locker:   locker,
		reporter: reporter,
		retry: retryPolicy{
			attempts: 3,
			base:     time.Second,
			max:      10 * time.Second,
		},
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

	err := s.runWithRetry(ctx, worker)
	if err != nil {
		// Only surface a failure once retries are exhausted. A transient
		// blip that recovers on a later attempt is not an operator-facing
		// failure, and counting every attempt would turn normal retries
		// into alert noise.
		s.log.Error("worker failed", "name", worker.Name(), "error", err)
		if s.reporter != nil {
			s.reporter.WorkerRunFailed(worker.Name())
		}
	}

	s.log.Debug("worker finished", "name", worker.Name(), "time", time.Since(start))
}

// runWithRetry runs the worker, retrying a failed attempt with exponential
// backoff up to the scheduler's retry policy. Retrying is safe only because
// earlier commits on this branch made every worker idempotent under re-run:
// the metrics-collect and application-health-check workers create a job only
// when no open job exists, the daily workers are DELETE-based, and the reaper
// only finishes stuck rows. Retry would not have been safe before that fix.
func (s *Scheduler) runWithRetry(ctx context.Context, worker Worker) error {
	attempts := s.retry.attempts
	if attempts < 1 {
		attempts = 1
	}

	var err error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 && !s.wait(ctx, retryDelay(attempt-1, s.retry.base, s.retry.max)) {
			// Shutdown during backoff: stop retrying and surface the last
			// failure to the caller.
			return err
		}

		err = worker.Run(ctx)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			// Shutdown must not keep retrying.
			return err
		}
		if attempt < attempts-1 {
			s.log.Warn("worker retry", "name", worker.Name(), "attempt", attempt+1, "max", attempts, "error", err)
		}
	}
	return err
}

// retryDelay is the wait before retry n (0-based): exponential from base,
// capped at max. No jitter: retries happen under the worker's advisory lock,
// so there is no stampede to spread out.
func retryDelay(n int, base, max time.Duration) time.Duration {
	if n < 0 {
		n = 0
	}
	d := base
	for i := 0; i < n; i++ {
		// Stop growing at the cap; further doubling would only risk
		// overflow for large n.
		if d >= max {
			return max
		}
		d *= 2
	}
	if d > max {
		return max
	}
	return d
}

// wait sleeps for d, returning false if ctx is cancelled first.
func (s *Scheduler) wait(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
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
