package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

func (r *UserRepository) List(ctx context.Context, opts domain.UserListOptions) ([]*domain.User, int64, error) {
	baseQuery := `
		SELECT
			u.id,
			u.name,
			u.email,
			u.role_id,
			u.created_at,
			u.updated_at,
			r.id,
			r.name
		FROM users u
		LEFT JOIN roles r ON u.role_id = r.id
	`

	args := []any{}
	conditions := []string{}
	argCounter := 1

	conditions = append(conditions, "u.deleted_at IS NULL")

	if opts.Search != "" {
		conditions = append(conditions, fmt.Sprintf("(u.name ILIKE $%d OR u.email ILIKE $%d)", argCounter, argCounter+1))
		searchParam := "%" + opts.Search + "%"
		args = append(args, searchParam, searchParam)
		argCounter += 2
	}

	if len(opts.Roles) > 0 {
		placeholders := []string{}
		for _, s := range opts.Roles {
			placeholders = append(placeholders, fmt.Sprintf("$%d", argCounter))
			args = append(args, s)
			argCounter++
		}
		conditions = append(conditions, fmt.Sprintf("r.name IN (%s)", strings.Join(placeholders, ", ")))
	}

	if len(conditions) > 0 {
		baseQuery += " WHERE " + strings.Join(conditions, " AND ")
	}

	baseQuery += " ORDER BY u.created_at ASC"

	var total int64
	if opts.IsPaginate {
		countQuery := "SELECT COUNT(*) FROM users u"
		if len(conditions) > 0 {
			countQuery += " WHERE " + strings.Join(conditions, " AND ")
		}
		if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("failed to count users: %w", err)
		}

		offset := (opts.Page - 1) * opts.Limit
		baseQuery += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argCounter, argCounter+1)
		args = append(args, opts.Limit, offset)
	} else {
		baseQuery += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}

	rows, err := r.db.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query users: %w", err)
	}
	defer rows.Close()

	var users []*domain.User
	for rows.Next() {
		var u domain.User
		var r domain.Role

		if err := rows.Scan(
			&u.ID,
			&u.Name,
			&u.Email,
			&u.RoleID,
			&u.CreatedAt,
			&u.UpdatedAt,
			&r.ID,
			&r.Name,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan users: %w", err)
		}

		u.Role = &r
		users = append(users, &u)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

func (r *UserRepository) GetByID(ctx context.Context, userID int64) (*domain.User, error) {
	query := `
		SELECT 
			u.id,
			u.name,
			u.email,
			u.password,
			u.role_id,
			u.created_at,
			u.updated_at,
			r.id,
			r.name
		FROM users u
		LEFT JOIN roles r ON u.role_id = r.id
		WHERE u.id = $1 AND u.deleted_at IS NULL
	`

	row := r.db.QueryRow(ctx, query, userID)

	var user domain.User
	var role domain.Role

	if err := row.Scan(
		&user.ID,
		&user.Name,
		&user.Email,
		&user.Password,
		&user.RoleID,
		&user.CreatedAt,
		&user.UpdatedAt,
		&role.ID,
		&role.Name,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}

	user.Role = &role
	return &user, nil
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	query := `
		SELECT 
			u.id,
			u.name,
			u.email,
			u.password,
			u.role_id,
			u.created_at,
			u.updated_at,
			r.id,
			r.name
		FROM users u
		LEFT JOIN roles r ON u.role_id = r.id
		WHERE u.email = $1 AND u.deleted_at IS NULL
	`

	row := r.db.QueryRow(ctx, query, email)

	var user domain.User
	var role domain.Role

	if err := row.Scan(
		&user.ID,
		&user.Name,
		&user.Email,
		&user.Password,
		&user.RoleID,
		&user.CreatedAt,
		&user.UpdatedAt,
		&role.ID,
		&role.Name,
	); err != nil {
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
		return fmt.Errorf("failed to update user: %w", err)
	}

	if ct.RowsAffected() == 0 {
		return fmt.Errorf("user with ID %d not found or deleted", userID)
	}

	return nil
}

func (r *UserRepository) Delete(ctx context.Context, userID int64) error {
	query := `UPDATE users SET deleted_at = $1 WHERE id = $2 AND deleted_at IS NULL`

	ct, err := r.db.Exec(ctx, query, time.Now().UTC(), userID)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	if ct.RowsAffected() == 0 {
		return fmt.Errorf("user with ID %d not found or already deleted", userID)
	}

	return nil
}
