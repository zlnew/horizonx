// Package domain
package domain

import (
	"context"
	"errors"
	"time"
)

var (
	ErrUserNotFound       = errors.New("user not found")
	ErrRoleNotFound       = errors.New("role not found")
	ErrEmailAlreadyExists = errors.New("email already exists")
)

type User struct {
	ID          int64        `json:"id"`
	Name        string       `json:"name"`
	Email       string       `json:"email"`
	Password    string       `json:"-"`
	RoleID      int64        `json:"role_id"`
	Role        *Role        `json:"role"`
	Permissions []Permission `json:"permissions"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
	DeletedAt   *time.Time   `json:"-"`
}

type Role struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type Permission struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type UserSaveRequest struct {
	Name     string `json:"name" validate:"required"`
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"omitempty,min=8"`
	RoleID   int64  `json:"role_id" validate:"required,numeric"`
}

type UserRepository interface {
	List(ctx context.Context, opts ListOptions) ([]*User, int64, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetByID(ctx context.Context, ID int64) (*User, error)
	GetRoleByID(ctx context.Context, roleID int64) (*Role, error)
	Create(ctx context.Context, user *User) error
	Update(ctx context.Context, user *User, userID int64) error
	Delete(ctx context.Context, userID int64) error
}

type UserService interface {
	List(ctx context.Context, opts ListOptions) (*ListResult[*User], error)
	Create(ctx context.Context, req UserSaveRequest) error
	Update(ctx context.Context, req UserSaveRequest, userID int64) error
	Delete(ctx context.Context, userID int64) error
}
