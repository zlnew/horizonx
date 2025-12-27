package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
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

func (r *DeploymentRepository) List(ctx context.Context, opts domain.DeploymentListOptions) ([]*domain.Deployment, int64, error) {
	baseQuery := `
		SELECT
			d.id,
			d.application_id,
			d.branch,
			d.commit_hash,
			d.commit_message,
			d.status,
			d.deployed_by,
			d.triggered_at,
			d.started_at,
			d.finished_at,
			u.id,
			u.name,
			u.email,
			u.role_id
		FROM deployments d
		LEFT JOIN users u ON d.deployed_by = u.id
	`

	args := []any{}
	conditions := []string{}
	argCounter := 1

	if opts.ApplicationID != nil {
		conditions = append(conditions, fmt.Sprintf("d.application_id = $%d", argCounter))
		args = append(args, *opts.ApplicationID)
		argCounter++
	}

	if opts.DeployedBy != nil {
		conditions = append(conditions, fmt.Sprintf("d.deployed_by = $%d", argCounter))
		args = append(args, *opts.DeployedBy)
		argCounter++
	}

	if len(opts.Statuses) > 0 {
		placeholders := []string{}
		for _, s := range opts.Statuses {
			placeholders = append(placeholders, fmt.Sprintf("$%d", argCounter))
			args = append(args, s)
			argCounter++
		}
		conditions = append(conditions, fmt.Sprintf("d.status IN (%s)", strings.Join(placeholders, ", ")))
	}

	if len(conditions) > 0 {
		baseQuery += " WHERE " + strings.Join(conditions, " AND ")
	}

	baseQuery += " ORDER BY d.triggered_at DESC"

	var total int64
	if opts.IsPaginate {
		countQuery := "SELECT COUNT(*) FROM deployments d"
		if len(conditions) > 0 {
			countQuery += " WHERE " + strings.Join(conditions, " AND ")
		}
		if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("failed to count deployments: %w", err)
		}

		offset := (opts.Page - 1) * opts.Limit
		baseQuery += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argCounter, argCounter+1)
		args = append(args, opts.Limit, offset)
	} else {
		baseQuery += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}

	rows, err := r.db.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query deployments: %w", err)
	}
	defer rows.Close()

	var deployments []*domain.Deployment
	for rows.Next() {
		var d domain.Deployment
		var (
			userID     sql.NullInt64
			userName   sql.NullString
			userEmail  sql.NullString
			userRoleID sql.NullInt64
		)

		if err := rows.Scan(
			&d.ID,
			&d.ApplicationID,
			&d.Branch,
			&d.CommitHash,
			&d.CommitMessage,
			&d.Status,
			&d.DeployedBy,
			&d.TriggeredAt,
			&d.StartedAt,
			&d.FinishedAt,
			&userID,
			&userName,
			&userEmail,
			&userRoleID,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan deployments: %w", err)
		}

		if userID.Valid {
			d.Deployer = &domain.User{
				ID:     userID.Int64,
				Name:   userName.String,
				Email:  userEmail.String,
				RoleID: userRoleID.Int64,
			}
		}

		deployments = append(deployments, &d)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return deployments, total, nil
}

func (r *DeploymentRepository) GetByID(ctx context.Context, deploymentID int64) (*domain.Deployment, error) {
	query := `
		SELECT
			d.id,
			d.application_id,
			d.branch,
			d.commit_hash,
			d.commit_message,
			d.status, 
			d.deployed_by,
			d.triggered_at,
			d.started_at,
			d.finished_at,
			u.id,
			u.name,
			u.email,
			u.role_id
		FROM deployments d
		LEFT JOIN users u ON d.deployed_by = u.id
		WHERE d.id = $1
	`

	var d domain.Deployment
	var (
		uID     sql.NullInt64
		uName   sql.NullString
		uEmail  sql.NullString
		uRoleID sql.NullInt64
	)

	if err := r.db.QueryRow(ctx, query, deploymentID).Scan(
		&d.ID,
		&d.ApplicationID,
		&d.Branch,
		&d.CommitHash,
		&d.CommitMessage,
		&d.Status,
		&d.DeployedBy,
		&d.TriggeredAt,
		&d.StartedAt,
		&d.FinishedAt,
		&uID,
		&uName,
		&uEmail,
		&uRoleID,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrDeploymentNotFound
		}
		return nil, fmt.Errorf("failed to scan deployment: %w", err)
	}

	if uID.Valid {
		d.Deployer = &domain.User{
			ID:     uID.Int64,
			Name:   uName.String,
			Email:  uEmail.String,
			RoleID: uRoleID.Int64,
		}
	}

	return &d, nil
}

func (r *DeploymentRepository) Create(ctx context.Context, d *domain.Deployment) (*domain.Deployment, error) {
	query := `
		INSERT INTO deployments (
			application_id,
			branch,
			deployed_by,
			status,
			triggered_at
		)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING
			id,
			application_id,
			deployed_by,
			triggered_at
	`

	now := time.Now().UTC()
	if err := r.db.QueryRow(ctx, query,
		d.ApplicationID,
		d.Branch,
		d.DeployedBy,
		domain.DeploymentPending,
		now,
	).Scan(
		&d.ID,
		&d.ApplicationID,
		&d.DeployedBy,
		&d.TriggeredAt,
	); err != nil {
		return nil, fmt.Errorf("failed to create deployment: %w", err)
	}

	return d, nil
}

func (r *DeploymentRepository) Start(ctx context.Context, deploymentID int64) (*domain.Deployment, error) {
	query := `
		UPDATE deployments 
		SET started_at = $1
		WHERE id = $2
		RETURNING
			id,
			application_id,
			started_at
	`

	var d domain.Deployment
	now := time.Now().UTC()
	if err := r.db.QueryRow(ctx, query, now, deploymentID).Scan(
		&d.ID,
		&d.ApplicationID,
		&d.StartedAt,
	); err != nil {
		return nil, fmt.Errorf("failed to start deployment: %w", err)
	}

	return &d, nil
}

func (r *DeploymentRepository) Finish(ctx context.Context, deploymentID int64) (*domain.Deployment, error) {
	query := `
		UPDATE deployments 
		SET finished_at = $1
		WHERE id = $2
		RETURNING
			id,
			application_id,
			status,
			finished_at
	`

	var d domain.Deployment
	now := time.Now().UTC()
	if err := r.db.QueryRow(ctx, query, now, deploymentID).Scan(
		&d.ID,
		&d.ApplicationID,
		&d.Status,
		&d.FinishedAt,
	); err != nil {
		return nil, fmt.Errorf("failed to finish deployment: %w", err)
	}

	return &d, nil
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

func (r *DeploymentRepository) UpdateCommitInfo(ctx context.Context, deploymentID int64, commitHash string, commitMessage string) (*domain.Deployment, error) {
	query := `
		UPDATE deployments
		SET commit_hash = $1, commit_message = $2
		WHERE id = $3
		RETURNING id, application_id, commit_hash, commit_message
	`

	var d domain.Deployment
	err := r.db.QueryRow(ctx, query, commitHash, commitMessage, deploymentID).Scan(
		&d.ID,
		&d.ApplicationID,
		&d.CommitHash,
		&d.CommitMessage,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.ErrDeploymentNotFound
		}
		return nil, fmt.Errorf("failed to update deployment commit info: %w", err)
	}

	return &d, nil
}
