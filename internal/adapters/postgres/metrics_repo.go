package postgres

import (
	"context"

	"horizonx-server/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MetricsRepository struct {
	db *pgxpool.Pool
}

func NewMetricsRepository(db *pgxpool.Pool) domain.MetricsRepository {
	return &MetricsRepository{db: db}
}

func (r *MetricsRepository) BulkInsert(ctx context.Context, metrics []domain.Metrics) error {
	if len(metrics) == 0 {
		return nil
	}

	rows := make([][]any, len(metrics))
	for i, m := range metrics {
		rows[i] = []any{
			m.ServerID,
			m.CPU.Usage,
			m.Memory.UsagePercent,
			m,
			m.RecordedAt,
		}
	}

	_, err := r.db.CopyFrom(
		ctx,
		pgx.Identifier{"server_metrics"},
		[]string{"server_id", "cpu_usage_percent", "memory_usage_percent", "data", "recorded_at"},
		pgx.CopyFromRows(rows),
	)

	return err
}
