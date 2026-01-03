package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"horizonx-server/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ApplicationRepository struct {
	db *pgxpool.Pool
}

func NewApplicationRepository(db *pgxpool.Pool) domain.ApplicationRepository {
	return &ApplicationRepository{db: db}
}

func (r *ApplicationRepository) List(ctx context.Context, opts domain.ApplicationListOptions) ([]*domain.Application, int64, error) {
	baseQuery := `
		SELECT
			id,
			server_id,
			name,
			repo_url,
			branch,
			status,
			last_deployment_at,
			created_at,
			updated_at
		FROM applications
	`

	args := []any{}
	conditions := []string{}
	argCounter := 1

	conditions = append(conditions, "deleted_at IS NULL")

	if opts.Search != "" {
		conditions = append(conditions, fmt.Sprintf("(name ILIKE $%d)", argCounter))
		searchParam := "%" + opts.Search + "%"
		args = append(args, searchParam)
		argCounter++
	}

	if opts.ServerID != nil {
		conditions = append(conditions, fmt.Sprintf("server_id = $%d", argCounter))
		args = append(args, *opts.ServerID)
		argCounter++
	}

	if len(conditions) > 0 {
		baseQuery += " WHERE " + strings.Join(conditions, " AND ")
	}

	baseQuery += " ORDER BY created_at DESC"

	var total int64
	if opts.IsPaginate {
		countQuery := "SELECT COUNT(*) FROM applications"
		if len(conditions) > 0 {
			countQuery += " WHERE " + strings.Join(conditions, " AND ")
		}
		if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("failed to count applications: %w", err)
		}

		offset := (opts.Page - 1) * opts.Limit
		baseQuery += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argCounter, argCounter+1)
		args = append(args, opts.Limit, offset)
	} else {
		baseQuery += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}

	rows, err := r.db.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query applications: %w", err)
	}
	defer rows.Close()

	var applications []*domain.Application
	for rows.Next() {
		var a domain.Application

		if err := rows.Scan(
			&a.ID,
			&a.ServerID,
			&a.Name,
			&a.RepoURL,
			&a.Branch,
			&a.Status,
			&a.LastDeploymentAt,
			&a.CreatedAt,
			&a.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan applications: %w", err)
		}

		applications = append(applications, &a)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return applications, total, nil
}

func (r *ApplicationRepository) GetByID(ctx context.Context, appID int64) (*domain.Application, error) {
	query := `
		SELECT id, server_id, name, repo_url, branch, status, last_deployment_at, created_at, updated_at
		FROM applications
		WHERE id = $1 AND deleted_at IS NULL
	`

	var app domain.Application
	err := r.db.QueryRow(ctx, query, appID).Scan(
		&app.ID,
		&app.ServerID,
		&app.Name,
		&app.RepoURL,
		&app.Branch,
		&app.Status,
		&app.LastDeploymentAt,
		&app.CreatedAt,
		&app.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrApplicationNotFound
		}
		return nil, fmt.Errorf("failed to get application: %w", err)
	}

	return &app, nil
}

func (r *ApplicationRepository) Create(ctx context.Context, app *domain.Application) (*domain.Application, error) {
	query := `
		INSERT INTO applications (server_id, name, repo_url, branch, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at
	`

	now := time.Now().UTC()
	err := r.db.QueryRow(
		ctx, query,
		app.ServerID,
		app.Name,
		app.RepoURL,
		app.Branch,
		domain.AppStatusStopped,
		now,
		now,
	).Scan(&app.ID, &app.CreatedAt, &app.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create application: %w", err)
	}

	return app, nil
}

func (r *ApplicationRepository) Update(ctx context.Context, app *domain.Application, appID int64) error {
	query := `
		UPDATE applications
		SET name = $1, repo_url = $2, branch = $3, updated_at = $4
		WHERE id = $5 AND deleted_at IS NULL
	`

	now := time.Now().UTC()
	ct, err := r.db.Exec(ctx, query,
		app.Name,
		app.RepoURL,
		app.Branch,
		now,
		appID,
	)
	if err != nil {
		return fmt.Errorf("failed to update application: %w", err)
	}

	if ct.RowsAffected() == 0 {
		return domain.ErrApplicationNotFound
	}

	return nil
}

func (r *ApplicationRepository) Delete(ctx context.Context, appID int64) error {
	query := `UPDATE applications SET deleted_at = $1 WHERE id = $2 AND deleted_at IS NULL`

	ct, err := r.db.Exec(ctx, query, time.Now().UTC(), appID)
	if err != nil {
		return fmt.Errorf("failed to delete application: %w", err)
	}

	if ct.RowsAffected() == 0 {
		return domain.ErrApplicationNotFound
	}

	return nil
}

func (r *ApplicationRepository) UpdateStatus(ctx context.Context, appID int64, status domain.ApplicationStatus) error {
	query := `
		UPDATE applications 
		SET status = $1, updated_at = $2
		WHERE id = $3 AND deleted_at IS NULL
	`

	ct, err := r.db.Exec(ctx, query, status, time.Now().UTC(), appID)
	if err != nil {
		return fmt.Errorf("failed to update application status: %w", err)
	}

	if ct.RowsAffected() == 0 {
		return domain.ErrApplicationNotFound
	}

	return nil
}

func (r *ApplicationRepository) UpdateLastDeployment(ctx context.Context, appID int64) error {
	query := `
		UPDATE applications
		SET last_deployment_at = $1
		WHERE id = $2 AND deleted_at IS NULL
	`

	ct, err := r.db.Exec(ctx, query, time.Now().UTC(), appID)
	if err != nil {
		return fmt.Errorf("failed to update application last deployment: %w", err)
	}

	if ct.RowsAffected() == 0 {
		return domain.ErrApplicationNotFound
	}

	return nil
}

func (r *ApplicationRepository) SyncEnvVars(ctx context.Context, appID int64, envVars []domain.EnvironmentVariable) error {
	if len(envVars) == 0 {
		return nil
	}

	now := time.Now().UTC()

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	batch := &pgx.Batch{}
	keys := make([]string, 0, len(envVars))

	for _, e := range envVars {
		keys = append(keys, e.Key)

		batch.Queue(`
			INSERT INTO environment_variables (
				application_id,
				key,
				value,
				is_preview,
				created_at,
				updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (application_id, key)
			DO UPDATE SET
				value = EXCLUDED.value,
				is_preview = EXCLUDED.is_preview,
				updated_at = EXCLUDED.updated_at
		`,
			appID,
			e.Key,
			e.Value,
			e.IsPreview,
			now,
			now,
		)
	}

	br := tx.SendBatch(ctx, batch)
	if _, err := br.Exec(); err != nil {
		br.Close()
		return fmt.Errorf("failed to upsert env vars: %w", err)
	}
	br.Close()

	_, err = tx.Exec(ctx, `
		DELETE FROM environment_variables
		WHERE application_id = $1
		  AND key NOT IN (SELECT unnest($2::text[]))
	`,
		appID,
		keys,
	)
	if err != nil {
		return fmt.Errorf("failed to delete stale env vars: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit tx: %w", err)
	}

	return nil
}

func (r *ApplicationRepository) ListEnvVars(ctx context.Context, appID int64) ([]domain.EnvironmentVariable, error) {
	query := `
		SELECT id, application_id, key, value, is_preview, created_at, updated_at
		FROM environment_variables
		WHERE application_id = $1
		ORDER BY key ASC
	`

	rows, err := r.db.Query(ctx, query, appID)
	if err != nil {
		return nil, fmt.Errorf("failed to query env vars: %w", err)
	}
	defer rows.Close()

	var envVars []domain.EnvironmentVariable
	for rows.Next() {
		var env domain.EnvironmentVariable
		err := rows.Scan(
			&env.ID,
			&env.ApplicationID,
			&env.Key,
			&env.Value,
			&env.IsPreview,
			&env.CreatedAt,
			&env.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan env var: %w", err)
		}
		envVars = append(envVars, env)
	}

	return envVars, nil
}

func (r *ApplicationRepository) CreateEnvVar(ctx context.Context, env *domain.EnvironmentVariable) error {
	query := `
		INSERT INTO environment_variables (application_id, key, value, is_preview, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (application_id, key) 
		DO UPDATE SET value = EXCLUDED.value, is_preview = EXCLUDED.is_preview, updated_at = EXCLUDED.updated_at
		RETURNING id, created_at, updated_at
	`

	now := time.Now().UTC()
	err := r.db.QueryRow(ctx, query,
		env.ApplicationID,
		env.Key,
		env.Value,
		env.IsPreview,
		now,
		now,
	).Scan(&env.ID, &env.CreatedAt, &env.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to create env var: %w", err)
	}

	return nil
}

func (r *ApplicationRepository) UpdateEnvVar(ctx context.Context, env *domain.EnvironmentVariable) error {
	query := `
		UPDATE environment_variables
		SET value = $1, is_preview = $2, updated_at = $3
		WHERE application_id = $4 AND key = $5
	`

	ct, err := r.db.Exec(ctx, query,
		env.Value,
		env.IsPreview,
		time.Now().UTC(),
		env.ApplicationID,
		env.Key,
	)
	if err != nil {
		return fmt.Errorf("failed to update env var: %w", err)
	}

	if ct.RowsAffected() == 0 {
		return fmt.Errorf("env var not found")
	}

	return nil
}

func (r *ApplicationRepository) DeleteEnvVar(ctx context.Context, appID int64, key string) error {
	query := `DELETE FROM environment_variables WHERE application_id = $1 AND key = $2`

	ct, err := r.db.Exec(ctx, query, appID, key)
	if err != nil {
		return fmt.Errorf("failed to delete env var: %w", err)
	}

	if ct.RowsAffected() == 0 {
		return fmt.Errorf("env var not found")
	}

	return nil
}

func (r *ApplicationRepository) UpdateHealth(ctx context.Context, serverID uuid.UUID, reports []domain.ApplicationHealth) error {
	if len(reports) == 0 {
		return nil
	}

	valueStrings := make([]string, 0, len(reports))
	valueArgs := make([]any, 0, len(reports)*2)

	argPos := 2
	for _, d := range reports {
		valueStrings = append(valueStrings, fmt.Sprintf("($%d::bigint, $%d::text)", argPos, argPos+1))
		valueArgs = append(valueArgs, d.ApplicationID, d.Status)
		argPos += 2
	}

	query := fmt.Sprintf(`
		UPDATE applications a
		SET status = v.status
		FROM (VALUES %s) AS v(app_id, status)
		WHERE a.id = v.app_id
		  AND a.server_id = $1
	`, strings.Join(valueStrings, ","))

	args := append([]any{serverID}, valueArgs...)

	_, err := r.db.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update applications health: %w", err)
	}

	return nil
}
