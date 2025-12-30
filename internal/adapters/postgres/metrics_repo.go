package postgres

import (
	"context"
	"fmt"
	"time"

	"horizonx-server/internal/domain"

	"github.com/google/uuid"
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
			m.CPU.Usage.EMA,
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

func (r *MetricsRepository) Cleanup(ctx context.Context, serverID uuid.UUID, cutoff time.Time) error {
	query := `
		DELETE FROM server_metrics
		WHERE server_id = $1
		AND recorded_at <= $2
	`

	_, err := r.db.Exec(ctx, query, serverID, cutoff)
	if err != nil {
		return fmt.Errorf("failed to cleanup server metrics: %w", err)
	}

	return nil
}
