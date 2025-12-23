package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"horizonx-server/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DeploymentRepository struct {
	db *pgxpool.Pool
}

func NewDeploymentRepository(db *pgxpool.Pool) domain.DeploymentRepository {
	return &DeploymentRepository{db: db}
}

func (r *DeploymentRepository) List(ctx context.Context, appID int64, limit int) ([]domain.Deployment, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT id, application_id, commit_hash, commit_message, status, 
		       build_logs, started_at, finished_at
		FROM deployments
		WHERE application_id = $1
		ORDER BY started_at DESC
		LIMIT $2
	`

	rows, err := r.db.Query(ctx, query, appID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query deployments: %w", err)
	}
	defer rows.Close()

	var deployments []domain.Deployment
	for rows.Next() {
		var d domain.Deployment
		err := rows.Scan(
			&d.ID,
			&d.ApplicationID,
			&d.CommitHash,
			&d.CommitMessage,
			&d.Status,
			&d.BuildLogs,
			&d.StartedAt,
			&d.FinishedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan deployment: %w", err)
		}
		deployments = append(deployments, d)
	}

	return deployments, nil
}

func (r *DeploymentRepository) GetByID(ctx context.Context, deploymentID int64) (*domain.Deployment, error) {
	query := `
		SELECT id, application_id, commit_hash, commit_message, status, 
		       build_logs, started_at, finished_at
		FROM deployments
		WHERE id = $1
	`

	var d domain.Deployment
	err := r.db.QueryRow(ctx, query, deploymentID).Scan(
		&d.ID,
		&d.ApplicationID,
		&d.CommitHash,
		&d.CommitMessage,
		&d.Status,
		&d.BuildLogs,
		&d.StartedAt,
		&d.FinishedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrDeploymentNotFound
		}
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}

	return &d, nil
}

func (r *DeploymentRepository) GetLatest(ctx context.Context, appID int64) (*domain.Deployment, error) {
	query := `
		SELECT id, application_id, commit_hash, commit_message, status, 
		       build_logs, started_at, finished_at
		FROM deployments
		WHERE application_id = $1
		ORDER BY started_at DESC
		LIMIT 1
	`

	var d domain.Deployment
	err := r.db.QueryRow(ctx, query, appID).Scan(
		&d.ID,
		&d.ApplicationID,
		&d.CommitHash,
		&d.CommitMessage,
		&d.Status,
		&d.BuildLogs,
		&d.StartedAt,
		&d.FinishedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrDeploymentNotFound
		}
		return nil, fmt.Errorf("failed to get latest deployment: %w", err)
	}

	return &d, nil
}

func (r *DeploymentRepository) Create(ctx context.Context, deployment *domain.Deployment) (*domain.Deployment, error) {
	query := `
		INSERT INTO deployments (application_id, branch, commit_hash, commit_message, deployed_by, status, started_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, started_at
	`

	now := time.Now().UTC()
	err := r.db.QueryRow(
		ctx, query,
		deployment.ApplicationID,
		deployment.Branch,
		deployment.CommitHash,
		deployment.CommitMessage,
		deployment.DeployedBy,
		domain.DeploymentPending,
		now,
	).Scan(&deployment.ID, &deployment.StartedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create deployment: %w", err)
	}

	return deployment, nil
}

func (r *DeploymentRepository) UpdateStatus(ctx context.Context, deploymentID int64, status domain.DeploymentStatus) (*domain.Deployment, error) {
	query := `
		UPDATE deployments
		SET status = $1
		WHERE id = $2
		RETURNING id, application_id, status
	`

	var d domain.Deployment
	err := r.db.QueryRow(ctx, query, status, deploymentID).Scan(
		&d.ID,
		&d.ApplicationID,
		&d.Status,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrDeploymentNotFound
		}
		return nil, fmt.Errorf("failed to update deployment status: %w", err)
	}

	return &d, nil
}

func (r *DeploymentRepository) UpdateCommitInfo(ctx context.Context, deploymentID int64, commitHash, commitMessage string) error {
	query := `UPDATE deployments SET commit_hash = $1, commit_message = $2 WHERE id = $3`

	ct, err := r.db.Exec(ctx, query, commitHash, commitMessage, deploymentID)
	if err != nil {
		return fmt.Errorf("failed to update commit info: %w", err)
	}

	if ct.RowsAffected() == 0 {
		return domain.ErrDeploymentNotFound
	}

	return nil
}

func (r *DeploymentRepository) UpdateLogs(ctx context.Context, deploymentID int64, logs string) (*domain.Deployment, error) {
	query := `
		UPDATE deployments
		SET build_logs = $1
		WHERE id = $2
		RETURNING id, application_id, build_logs
	`

	var d domain.Deployment
	err := r.db.QueryRow(ctx, query, logs, deploymentID).Scan(
		&d.ID,
		&d.ApplicationID,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrDeploymentNotFound
		}
		return nil, fmt.Errorf("failed to update deployment build logs: %w", err)
	}

	return &d, nil
}

func (r *DeploymentRepository) Finish(ctx context.Context, deploymentID int64, status domain.DeploymentStatus, logs string) error {
	query := `
		UPDATE deployments 
		SET status = $1, build_logs = $2, finished_at = $3
		WHERE id = $4
	`

	now := time.Now().UTC()
	ct, err := r.db.Exec(ctx, query, status, logs, now, deploymentID)
	if err != nil {
		return fmt.Errorf("failed to finish deployment: %w", err)
	}

	if ct.RowsAffected() == 0 {
		return domain.ErrDeploymentNotFound
	}

	return nil
}
