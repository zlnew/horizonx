package postgres

import (
	"context"
	"fmt"
	"time"

	"horizonx/internal/domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RetentionRepository implements domain.RetentionRepository with batched,
// oldest-first deletes so a huge first purge does not hold one long lock.
type RetentionRepository struct {
	db *pgxpool.Pool
}

func NewRetentionRepository(db *pgxpool.Pool) domain.RetentionRepository {
	return &RetentionRepository{db: db}
}

func (r *RetentionRepository) DeleteJobsOlderThan(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM jobs
		WHERE ctid IN (
			SELECT ctid FROM jobs
			WHERE queued_at < $1
			ORDER BY queued_at
			LIMIT $2
		)`,
		cutoff, limit,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired jobs: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (r *RetentionRepository) DeleteLogsOlderThan(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM logs
		WHERE ctid IN (
			SELECT ctid FROM logs
			WHERE timestamp < $1
			ORDER BY timestamp
			LIMIT $2
		)`,
		cutoff, limit,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired logs: %w", err)
	}
	return tag.RowsAffected(), nil
}
