package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"horizonx-server/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type JobRepository struct {
	db *pgxpool.Pool
}

func NewJobRepository(db *pgxpool.Pool) domain.JobRepository {
	return &JobRepository{db: db}
}

func (r *JobRepository) List(ctx context.Context, opts domain.JobListOptions) ([]*domain.Job, int64, error) {
	baseQuery := `
		SELECT
			id,
			trace_id,
			server_id,
			application_id,
			deployment_id,
			type,
			payload,
			status,
			queued_at,
			started_at,
			finished_at,
			expired_at
		FROM jobs
	`

	args := []any{}
	conditions := []string{}
	argCounter := 1

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

	if opts.Type != "" {
		conditions = append(conditions, fmt.Sprintf("type = $%d", argCounter))
		args = append(args, opts.Type)
		argCounter++
	}

	if len(opts.Statuses) > 0 {
		placeholders := []string{}
		for _, s := range opts.Statuses {
			placeholders = append(placeholders, fmt.Sprintf("$%d", argCounter))
			args = append(args, s)
			argCounter++
		}
		conditions = append(conditions, fmt.Sprintf("status IN (%s)", strings.Join(placeholders, ", ")))
	}

	if len(conditions) > 0 {
		baseQuery += " WHERE " + strings.Join(conditions, " AND ")
	}

	baseQuery += " ORDER BY queued_at DESC"

	var total int64
	if opts.IsPaginate {
		countQuery := "SELECT COUNT(*) FROM jobs"
		if len(conditions) > 0 {
			countQuery += " WHERE " + strings.Join(conditions, " AND ")
		}
		if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("failed to count jobs: %w", err)
		}

		offset := (opts.Page - 1) * opts.Limit
		baseQuery += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argCounter, argCounter+1)
		args = append(args, opts.Limit, offset)
	} else {
		baseQuery += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}

	rows, err := r.db.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*domain.Job
	for rows.Next() {
		var job domain.Job
		if err := rows.Scan(
			&job.ID,
			&job.TraceID,
			&job.ServerID,
			&job.ApplicationID,
			&job.DeploymentID,
			&job.Type,
			&job.Payload,
			&job.Status,
			&job.QueuedAt,
			&job.StartedAt,
			&job.FinishedAt,
			&job.ExpiredAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan jobs: %w", err)
		}
		jobs = append(jobs, &job)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return jobs, total, nil
}

func (r *JobRepository) GetPending(ctx context.Context, serverID uuid.UUID) ([]*domain.Job, error) {
	query := `
		SELECT
			id,
			trace_id,
			server_id,
			application_id,
			deployment_id,
			type,
			payload,
			status,
			queued_at,
			expired_at
		FROM jobs
		WHERE server_id = $1
		AND status = $2
		ORDER BY queued_at ASC
		LIMIT 30
	`

	rows, err := r.db.Query(ctx, query, serverID, domain.JobQueued)
	if err != nil {
		return nil, err
	}

	var jobs []*domain.Job
	for rows.Next() {
		var j domain.Job
		if err := rows.Scan(
			&j.ID,
			&j.TraceID,
			&j.ServerID,
			&j.ApplicationID,
			&j.DeploymentID,
			&j.Type,
			&j.Payload,
			&j.Status,
			&j.QueuedAt,
			&j.ExpiredAt,
		); err != nil {
			return nil, err
		}

		jobs = append(jobs, &j)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return jobs, nil
}

func (r *JobRepository) GetByID(ctx context.Context, jobID int64) (*domain.Job, error) {
	query := `
		SELECT
			id,
			trace_id,
			server_id,
			application_id,
			deployment_id,
			type,
			payload,
			status,
			queued_at,
			started_at,
			finished_at,
			expired_at
		FROM jobs
		WHERE id = $1 LIMIT 1
	`

	var j domain.Job
	err := r.db.QueryRow(ctx, query, jobID).Scan(
		&j.ID,
		&j.TraceID,
		&j.ServerID,
		&j.ApplicationID,
		&j.DeploymentID,
		&j.Type,
		&j.Payload,
		&j.Status,
		&j.QueuedAt,
		&j.StartedAt,
		&j.FinishedAt,
		&j.ExpiredAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrJobNotFound
		}

		return nil, err
	}

	return &j, nil
}

func (r *JobRepository) Create(ctx context.Context, j *domain.Job) (*domain.Job, error) {
	query := `
		INSERT INTO jobs
		(
			trace_id,
			server_id,
			application_id,
			deployment_id,
			type,
			payload,
			expired_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, queued_at
	`

	err := r.db.QueryRow(ctx, query,
		j.TraceID,
		j.ServerID,
		j.ApplicationID,
		j.DeploymentID,
		j.Type,
		j.Payload,
		j.ExpiredAt,
	).Scan(
		&j.ID,
		&j.QueuedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create job: %w", err)
	}

	return j, nil
}

func (r *JobRepository) Delete(ctx context.Context, jobID int64) error {
	query := `
		DELETE FROM jobs
		WHERE id = $1
		RETURNING id
	`

	var deletedID int64
	err := r.db.QueryRow(ctx, query, jobID).Scan(&deletedID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrJobNotFound
		}
		return err
	}

	return nil
}

func (r *JobRepository) Retry(ctx context.Context, jobID int64, j *domain.Job) (*domain.Job, error) {
	query := `
		UPDATE jobs
		SET
			status = $2,
			queued_at = $3,
			started_at = null,
			finished_at = null,
			expired_at = $4
		WHERE id = $1
		RETURNING
			id,
			trace_id,
			server_id,
			application_id,
			deployment_id,
			type,
			payload,
			status,
			queued_at,
			expired_at
	`

	err := r.db.QueryRow(ctx, query,
		jobID,
		j.Status,
		j.QueuedAt,
		j.ExpiredAt,
	).Scan(
		&j.ID,
		&j.TraceID,
		&j.ServerID,
		&j.ApplicationID,
		&j.DeploymentID,
		&j.Type,
		&j.Payload,
		&j.Status,
		&j.QueuedAt,
		&j.ExpiredAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to retry job: %w", err)
	}

	return j, nil
}

func (r *JobRepository) MarkRunning(ctx context.Context, jobID int64) (*domain.Job, error) {
	query := `
		UPDATE jobs
		SET
			status = 'running',
			started_at = NOW()
		WHERE id = $1
		  AND status = 'queued'
		RETURNING
			id,
			trace_id,
			server_id,
			application_id,
			deployment_id,
			type,
			payload,
			status,
			queued_at,
			started_at,
			finished_at,
			expired_at
	`

	var job domain.Job
	err := r.db.QueryRow(ctx, query, jobID).Scan(
		&job.ID,
		&job.TraceID,
		&job.ServerID,
		&job.ApplicationID,
		&job.DeploymentID,
		&job.Type,
		&job.Payload,
		&job.Status,
		&job.QueuedAt,
		&job.StartedAt,
		&job.FinishedAt,
		&job.ExpiredAt,
	)

	if err == pgx.ErrNoRows {
		return r.GetByID(ctx, jobID)
	}
	if err != nil {
		return nil, err
	}

	return &job, nil
}

func (r *JobRepository) MarkFinished(
	ctx context.Context,
	jobID int64,
	status domain.JobStatus,
) (*domain.Job, error) {
	query := `
		UPDATE jobs
		SET
			status = $1,
			finished_at = NOW()
		WHERE id = $2
		  AND status = 'running'
		RETURNING
			id,
			trace_id,
			server_id,
			application_id,
			deployment_id,
			type,
			payload,
			status,
			queued_at,
			started_at,
			finished_at,
			expired_at
	`

	var job domain.Job
	err := r.db.QueryRow(ctx, query, status, jobID).Scan(
		&job.ID,
		&job.TraceID,
		&job.ServerID,
		&job.ApplicationID,
		&job.DeploymentID,
		&job.Type,
		&job.Payload,
		&job.Status,
		&job.QueuedAt,
		&job.StartedAt,
		&job.FinishedAt,
		&job.ExpiredAt,
	)

	if err == pgx.ErrNoRows {
		return r.GetByID(ctx, jobID)
	}
	if err != nil {
		return nil, err
	}

	return &job, nil
}
