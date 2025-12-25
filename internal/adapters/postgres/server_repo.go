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

type ServerRepository struct {
	db *pgxpool.Pool
}

func NewServerRepository(db *pgxpool.Pool) domain.ServerRepository {
	return &ServerRepository{db: db}
}

func (r *ServerRepository) List(ctx context.Context, opts domain.ServerListOptions) ([]*domain.Server, int64, error) {
	baseQuery := `
		SELECT
			id,
			name,
			COALESCE(ip_address::text, ''),
			is_online,
			os_info,
			created_at,
			updated_at
		FROM servers
	`

	args := []any{}
	conditions := []string{}
	argCounter := 1

	conditions = append(conditions, "deleted_at IS NULL")

	if opts.Search != "" {
		conditions = append(conditions, fmt.Sprintf("(name ILIKE $%d OR ip_address::text ILIKE $%d)", argCounter, argCounter+1))
		searchParam := "%" + opts.Search + "%"
		args = append(args, searchParam, searchParam)
		argCounter += 2
	}

	if opts.IsOnline != nil {
		if *opts.IsOnline {
			conditions = append(conditions, "is_online IS TRUE")
		} else {
			conditions = append(conditions, "is_online IS FALSE")
		}
	}

	if len(conditions) > 0 {
		baseQuery += " WHERE " + strings.Join(conditions, " AND ")
	}

	baseQuery += " ORDER BY created_at ASC"

	var total int64
	if opts.IsPaginate {
		countQuery := "SELECT COUNT(*) FROM servers"
		if len(conditions) > 0 {
			countQuery += " WHERE " + strings.Join(conditions, " AND ")
		}
		if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("failed to count servers: %w", err)
		}

		offset := (opts.Page - 1) * opts.Limit
		baseQuery += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argCounter, argCounter+1)
		args = append(args, opts.Limit, offset)
	} else {
		baseQuery += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}

	rows, err := r.db.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query servers: %w", err)
	}
	defer rows.Close()

	var servers []*domain.Server
	for rows.Next() {
		var s domain.Server

		if err := rows.Scan(
			&s.ID,
			&s.Name,
			&s.IPAddress,
			&s.IsOnline,
			&s.OSInfo,
			&s.CreatedAt,
			&s.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan servers: %w", err)
		}

		servers = append(servers, &s)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return servers, total, nil
}

func (r *ServerRepository) GetByID(ctx context.Context, serverID uuid.UUID) (*domain.Server, error) {
	query := `
		SELECT
			id,
			name,
			COALESCE(ip_address::text, ''),
			api_token,
			is_online,
			os_info,
			created_at,
			updated_at
		FROM servers
		WHERE id = $1 AND deleted_at IS NULL LIMIT 1
	`

	var s domain.Server
	err := r.db.QueryRow(ctx, query, serverID).Scan(
		&s.ID,
		&s.Name,
		&s.IPAddress,
		&s.APIToken,
		&s.IsOnline,
		&s.OSInfo,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrServerNotFound
		}
		return nil, err
	}

	return &s, nil
}

func (r *ServerRepository) GetByToken(ctx context.Context, token string) (*domain.Server, error) {
	query := `
		SELECT
			id,
			name,
			COALESCE(ip_address::text, ''),
			is_online,
			os_info,
			created_at,
			updated_at
		FROM servers
		WHERE api_token = $1 AND deleted_at IS NULL LIMIT 1
	`

	var s domain.Server
	err := r.db.QueryRow(ctx, query, token).Scan(
		&s.ID,
		&s.Name,
		&s.IPAddress,
		&s.IsOnline,
		&s.OSInfo,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrServerNotFound
		}
		return nil, err
	}

	return &s, nil
}

func (r *ServerRepository) Create(ctx context.Context, s *domain.Server) (*domain.Server, error) {
	query := `
		INSERT INTO servers (name, ip_address, api_token, is_online, created_at, updated_at)
		VALUES ($1, $2::inet, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`

	var ipParam any = nil
	if s.IPAddress != "" {
		ipParam = s.IPAddress
	}

	now := time.Now().UTC()

	err := r.db.QueryRow(
		ctx,
		query,
		s.Name,
		ipParam,
		s.APIToken,
		s.IsOnline,
		now,
		now,
	).Scan(
		&s.ID,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create server: %w", err)
	}

	return s, nil
}

func (r *ServerRepository) Update(ctx context.Context, s *domain.Server, serverID uuid.UUID) error {
	query := `
		UPDATE servers
		SET name = $1, ip_address = $2, updated_at = $3
		WHERE id = $4 AND deleted_at IS NULL
	`

	now := time.Now().UTC()
	ct, err := r.db.Exec(ctx, query,
		s.Name,
		s.IPAddress,
		now,
		serverID,
	)
	if err != nil {
		return fmt.Errorf("failed to update server: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("server with ID %s not found or deleted", serverID.String())
	}

	return nil
}

func (r *ServerRepository) Delete(ctx context.Context, serverID uuid.UUID) error {
	query := `UPDATE servers SET deleted_at = $1 WHERE id = $2 AND deleted_at IS NULL`

	ct, err := r.db.Exec(ctx, query, time.Now().UTC(), serverID)
	if err != nil {
		return fmt.Errorf("failed to delete server: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("server with ID %s not found or already deleted", serverID.String())
	}

	return nil
}

func (r *ServerRepository) UpdateStatus(ctx context.Context, serverID uuid.UUID, isOnline bool) error {
	now := time.Now().UTC()
	query := `UPDATE servers SET is_online = $1, updated_at = $2 WHERE id = $3 AND deleted_at IS NULL`
	_, err := r.db.Exec(ctx, query, isOnline, now, serverID)
	return err
}
