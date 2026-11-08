package workers

import (
	"context"
	"time"

	"horizonx/internal/domain"
	"horizonx/internal/logger"
)

// RetentionBatchSize bounds the rows deleted per statement so the first
// purge against a long-accumulated table does not hold one huge lock.
// RetentionMaxIterations caps total batches per table per run so the
// worker cannot loop forever if rows keep arriving faster than purging.
const (
	RetentionBatchSize     = 1000
	RetentionMaxIterations = 100
)

type RetentionWorker struct {
	retention domain.RetentionRepository
	jobTTL    time.Duration
	logTTL    time.Duration
	log       logger.Logger
}

func NewRetentionWorker(retention domain.RetentionRepository, jobTTL, logTTL time.Duration, log logger.Logger) Worker {
	return &RetentionWorker{
		retention: retention,
		jobTTL:    jobTTL,
		logTTL:    logTTL,
		log:       log,
	}
}

func (w *RetentionWorker) Name() string {
	return "job_log_retention"
}

func (w *RetentionWorker) Run(ctx context.Context) error {
	now := time.Now().UTC()

	w.purgeTable(ctx, "jobs", now.Add(-w.jobTTL), func(cutoff time.Time, limit int) (int64, error) {
		return w.retention.DeleteJobsOlderThan(ctx, cutoff, limit)
	})
	w.purgeTable(ctx, "logs", now.Add(-w.logTTL), func(cutoff time.Time, limit int) (int64, error) {
		return w.retention.DeleteLogsOlderThan(ctx, cutoff, limit)
	})

	return nil
}

func (w *RetentionWorker) purgeTable(ctx context.Context, table string, cutoff time.Time, deleteBatch func(time.Time, int) (int64, error)) {
	var total int64
	for i := 0; i < RetentionMaxIterations; i++ {
		n, err := deleteBatch(cutoff, RetentionBatchSize)
		if err != nil {
			w.log.Error("retention purge failed", "table", table, "error", err.Error())
			return
		}
		total += n
		if n < int64(RetentionBatchSize) {
			w.log.Info("retention purge finished", "table", table, "deleted", total)
			return
		}
	}

	// The cap was reached without draining the table. Say so explicitly:
	// otherwise this is indistinguishable from a clean finish, and an operator
	// cannot tell that retention is failing to keep up with the backlog.
	w.log.Warn("retention purge hit the batch cap - table not fully drained",
		"table", table,
		"deleted", total,
		"batch_size", RetentionBatchSize,
		"max_iterations", RetentionMaxIterations,
	)
}
