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

type UserRepository struct {
	db *pgxpool.Pool
}

func NewUserRepository(db *pgxpool.Pool) domain.UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) List(ctx context.Context, opts domain.ListOptions) ([]*domain.User, int64, error) {
	baseQuery := `
		FROM users u
		LEFT JOIN roles r ON u.role_id = r.id
		WHERE u.deleted_at IS NULL
	`
	args := []any{}
	argCounter := 1

	if opts.Search != "" {
		baseQuery += fmt.Sprintf(" AND (u.email ILIKE $%d OR u.name ILIKE $%d)", argCounter, argCounter+1)
		searchParam := "%" + opts.Search + "%"
		args = append(args, searchParam, searchParam)
		argCounter += 2
	}

	var total int64
	if opts.IsPaginate {
		countQuery := "SELECT COUNT(*) " + baseQuery
		if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("failed to count users: %w", err)
		}
	}

	selectQuery := `
		SELECT 
			u.id, u.name, u.email, u.password, u.role_id, u.created_at, u.updated_at,
			r.id, r.name
	` + baseQuery

	selectQuery += " ORDER BY u.created_at DESC"

	if opts.IsPaginate {
		offset := (opts.Page - 1) * opts.Limit
		selectQuery += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argCounter, argCounter+1)
		args = append(args, opts.Limit, offset)
	} else {
		selectQuery += " LIMIT 1000"
	}

	rows, err := r.db.Query(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query users: %w", err)
	}
	defer rows.Close()

	var users []*domain.User
	for rows.Next() {
		var user domain.User
		var role domain.Role

		err := rows.Scan(
			&user.ID, &user.Name, &user.Email, &user.Password, &user.RoleID, &user.CreatedAt, &user.UpdatedAt,
			&role.ID, &role.Name,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scan failed: %w", err)
		}

		user.Role = &role
		users = append(users, &user)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

func (r *UserRepository) GetUserByID(ctx context.Context, ID int64) (*domain.User, error) {
	query := `
		SELECT 
			u.id, u.name, u.email, u.password, u.role_id, u.created_at, u.updated_at,
			r.id, r.name
		FROM users u
		LEFT JOIN roles r ON u.role_id = r.id
		WHERE u.id = $1 AND u.deleted_at IS NULL
	`

	row := r.db.QueryRow(ctx, query, ID)

	var user domain.User
	var role domain.Role

	err := row.Scan(
		&user.ID, &user.Name, &user.Email, &user.Password, &user.RoleID, &user.CreatedAt, &user.UpdatedAt,
		&role.ID, &role.Name,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}

	user.Role = &role
	return &user, nil
}

func (r *UserRepository) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	query := `
		SELECT 
			u.id, u.name, u.email, u.password, u.role_id, u.created_at, u.updated_at,
			r.id, r.name
		FROM users u
		LEFT JOIN roles r ON u.role_id = r.id
		WHERE u.email = $1 AND u.deleted_at IS NULL
	`

	row := r.db.QueryRow(ctx, query, email)

	var user domain.User
	var role domain.Role

	err := row.Scan(
		&user.ID, &user.Name, &user.Email, &user.Password, &user.RoleID, &user.CreatedAt, &user.UpdatedAt,
		&role.ID, &role.Name,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}

	user.Role = &role
	return &user, nil
}

func (r *UserRepository) GetRoleByID(ctx context.Context, roleID int64) (*domain.Role, error) {
	query := `SELECT id, name FROM roles WHERE id = $1`

	var role domain.Role
	err := r.db.QueryRow(ctx, query, roleID).Scan(&role.ID, &role.Name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrRoleNotFound
		}
		return nil, err
	}

	return &role, nil
}

func (r *UserRepository) Create(ctx context.Context, user *domain.User) error {
	query := `
		INSERT INTO users (name, email, password, role_id, created_at, updated_at) 
		VALUES ($1, $2, $3, $4, $5, $6) 
		RETURNING id
	`

	now := time.Now().UTC()

	err := r.db.QueryRow(ctx, query,
		user.Name,
		user.Email,
		user.Password,
		user.RoleID,
		now,
		now,
	).Scan(&user.ID)
	if err != nil {
		return fmt.Errorf("failed to insert user: %w", err)
	}

	user.CreatedAt = now
	user.UpdatedAt = now

	return nil
}

func (r *UserRepository) Update(ctx context.Context, user *domain.User, userID int64) error {
	query := `
		UPDATE users 
		SET name = $1, email = $2, password = $3, role_id = $4, updated_at = $5
		WHERE id = $6 AND deleted_at IS NULL
	`

	now := time.Now().UTC()
	ct, err := r.db.Exec(ctx, query,
		user.Name,
		user.Email,
		user.Password,
		user.RoleID,
		now,
		userID,
	)
	if err != nil {
		return fmt.Errorf("failed to execute update query: %w", err)
	}

	if ct.RowsAffected() == 0 {
		return fmt.Errorf("user with ID %d not found or deleted", userID)
	}

	user.UpdatedAt = now

	return nil
}

func (r *UserRepository) Delete(ctx context.Context, userID int64) error {
	query := `UPDATE users SET deleted_at = $1 WHERE id = $2 AND deleted_at IS NULL`

	ct, err := r.db.Exec(ctx, query, time.Now().UTC(), userID)
	if err != nil {
		return fmt.Errorf("failed to execute soft delete query: %w", err)
	}

	if ct.RowsAffected() == 0 {
		return fmt.Errorf("user with ID %d not found or already deleted", userID)
	}

	return nil
}
