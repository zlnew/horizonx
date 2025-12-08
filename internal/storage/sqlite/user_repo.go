package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"horizonx-server/internal/core/auth"
)

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) CreateUser(ctx context.Context, user *auth.User) error {
	query := `INSERT INTO users (email, password) VALUES (?, ?)`

	stmt, err := r.db.PrepareContext(ctx, query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.ExecContext(ctx, user.Email, user.Password)
	if err != nil {
		return fmt.Errorf("failed to insert user: %w", err)
	}
	return nil
}

func (r *UserRepository) GetUserByEmail(ctx context.Context, email string) (*auth.User, error) {
	query := `SELECT id, email, password FROM users WHERE email = ?`

	row := r.db.QueryRowContext(ctx, query, email)

	var user auth.User
	if err := row.Scan(&user.ID, &user.Email, &user.Password); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, err
	}
	return &user, nil
}
