package domain

import (
	"context"
	"time"
)

// RetentionRepository purges expired rows from the ever-growing jobs and
// logs tables. It is deliberately separate from JobRepository /
// LogRepository: those interfaces have checked-in generated mocks and
// mockery is not available, so extending them would break go vet repo-wide.
type RetentionRepository interface {
	DeleteJobsOlderThan(ctx context.Context, cutoff time.Time, limit int) (int64, error)
	DeleteLogsOlderThan(ctx context.Context, cutoff time.Time, limit int) (int64, error)
}
