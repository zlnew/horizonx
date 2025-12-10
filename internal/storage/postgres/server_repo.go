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

type ServerRepository struct {
	db *pgxpool.Pool
}

func NewServerRepository(db *pgxpool.Pool) domain.ServerRepository {
	return &ServerRepository{db: db}
}

func (r *ServerRepository) GetByToken(ctx context.Context, token string) (*domain.Server, error) {
	query := `
		SELECT id, name, COALESCE(ip_address::text, ''), is_online, os_info, created_at, updated_at
		FROM servers
		WHERE api_token = $1 LIMIT 1
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
			return nil, errors.New("invalid token")
		}
		return nil, fmt.Errorf("db error: %w", err)
	}

	return &s, nil
}

func (r *ServerRepository) Create(ctx context.Context, s *domain.Server) error {
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

	err := r.db.QueryRow(ctx, query, s.Name, ipParam, s.APIToken, s.IsOnline, now, now).Scan(
		&s.ID,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	return nil
}

func (r *ServerRepository) List(ctx context.Context) ([]domain.Server, error) {
	query := `
		SELECT id, name, COALESCE(ip_address::text, ''), is_online, os_info, created_at, updated_at
		FROM servers
		ORDER BY name ASC
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []domain.Server
	for rows.Next() {
		var s domain.Server
		err := rows.Scan(
			&s.ID,
			&s.Name,
			&s.IPAddress,
			&s.IsOnline,
			&s.OSInfo,
			&s.CreatedAt,
			&s.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		servers = append(servers, s)
	}

	return servers, nil
}

func (r *ServerRepository) UpdateStatus(ctx context.Context, id int64, isOnline bool) error {
	now := time.Now().UTC()
	query := `UPDATE servers SET is_online = $1, updated_at = $2 WHERE id = $3`
	_, err := r.db.Exec(ctx, query, isOnline, now, id)
	return err
}
