package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"horizonx-server/internal/domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

type LogRepository struct {
	db *pgxpool.Pool
}

func NewLogRepository(db *pgxpool.Pool) domain.LogRepository {
	return &LogRepository{db: db}
}

func (r *LogRepository) List(ctx context.Context, opts domain.LogListOptions) ([]*domain.Log, int64, error) {
	baseQuery := `
		SELECT
			id,
			timestamp,
			level,
			source,
			action,
			trace_id,
			job_id,
			server_id,
			application_id,
			deployment_id,
			message,
			context,
			created_at
		FROM logs
	`

	args := []any{}
	conditions := []string{}
	argCounter := 1

	if opts.TraceID != nil {
		conditions = append(conditions, fmt.Sprintf("trace_id = $%d", argCounter))
		args = append(args, *opts.TraceID)
		argCounter++
	}

	if opts.JobID != nil {
		conditions = append(conditions, fmt.Sprintf("job_id = $%d", argCounter))
		args = append(args, *opts.JobID)
		argCounter++
	}

	if opts.ServerID != nil {
		conditions = append(conditions, fmt.Sprintf("server_id = $%d", argCounter))
		args = append(args, *opts.ServerID)
		argCounter++
	}

	if opts.ApplicationID != nil {
		conditions = append(conditions, fmt.Sprintf("application_id = $%d", argCounter))
		args = append(args, *opts.ApplicationID)
		argCounter++
	}

	if opts.DeploymentID != nil {
		conditions = append(conditions, fmt.Sprintf("deployment_id = $%d", argCounter))
		args = append(args, *opts.DeploymentID)
		argCounter++
	}

	if len(opts.Levels) > 0 {
		placeholders := []string{}
		for _, s := range opts.Levels {
			placeholders = append(placeholders, fmt.Sprintf("$%d", argCounter))
			args = append(args, s)
			argCounter++
		}
		conditions = append(conditions, fmt.Sprintf("level IN (%s)", strings.Join(placeholders, ", ")))
	}

	if len(opts.Sources) > 0 {
		placeholders := []string{}
		for _, s := range opts.Levels {
			placeholders = append(placeholders, fmt.Sprintf("$%d", argCounter))
			args = append(args, s)
			argCounter++
		}
		conditions = append(conditions, fmt.Sprintf("source IN (%s)", strings.Join(placeholders, ", ")))
	}

	if len(opts.Actions) > 0 {
		placeholders := []string{}
		for _, s := range opts.Levels {
			placeholders = append(placeholders, fmt.Sprintf("$%d", argCounter))
			args = append(args, s)
			argCounter++
		}
		conditions = append(conditions, fmt.Sprintf("action IN (%s)", strings.Join(placeholders, ", ")))
	}

	if len(conditions) > 0 {
		baseQuery += " WHERE " + strings.Join(conditions, " AND ")
	}

	baseQuery += " ORDER BY timestamp ASC"

	var total int64
	if opts.IsPaginate {
		countQuery := "SELECT COUNT(*) FROM logs"
		if len(conditions) > 0 {
			countQuery += " WHERE " + strings.Join(conditions, " AND ")
		}
		if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("failed to count logs: %w", err)
		}

		offset := (opts.Page - 1) * opts.Limit
		baseQuery += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argCounter, argCounter+1)
		args = append(args, opts.Limit, offset)
	} else {
		baseQuery += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}

	rows, err := r.db.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query logs: %w", err)
	}
	defer rows.Close()

	var logs []*domain.Log
	for rows.Next() {
		var l domain.Log

		if err := rows.Scan(
			&l.ID,
			&l.Timestamp,
			&l.Level,
			&l.Source,
			&l.Action,
			&l.TraceID,
			&l.JobID,
			&l.ServerID,
			&l.ApplicationID,
			&l.DeploymentID,
			&l.Message,
			&l.Context,
			&l.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan logs: %w", err)
		}

		logs = append(logs, &l)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

func (r *LogRepository) Create(ctx context.Context, l *domain.Log) (*domain.Log, error) {
	query := `
		INSERT INTO logs
		(
			timestamp,
			level,
			source,
			action,
			trace_id,
			job_id,
			server_id,
			application_id,
			deployment_id,
			message,
			context,
			created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING
			id,
			timestamp,
			level,
			source,
			action,
			trace_id,
			job_id,
			server_id,
			application_id,
			deployment_id,
			message,
			context,
			created_at
	`

	now := time.Now().UTC()
	err := r.db.QueryRow(ctx, query,
		l.Timestamp,
		l.Level,
		l.Source,
		l.Action,
		l.TraceID,
		l.JobID,
		l.ServerID,
		l.ApplicationID,
		l.DeploymentID,
		l.Message,
		l.Context,
		now,
	).Scan(
		&l.ID,
		&l.Timestamp,
		&l.Level,
		&l.Source,
		&l.Action,
		&l.TraceID,
		&l.JobID,
		&l.ServerID,
		&l.ApplicationID,
		&l.DeploymentID,
		&l.Message,
		&l.Context,
		&l.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to emit logs: %w", err)
	}

	return l, nil
}
