// Package domain
package domain

import (
	"context"
	"errors"
)

var (
	ErrUserNotFound       = errors.New("user not found")
	ErrEmailAlreadyExists = errors.New("email already exists")
)

type User struct {
	ID       int64  `json:"id"`
	Email    string `json:"email"`
	Password string `json:"-"`
}

type UserSaveRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type UserRepository interface {
	GetUsers(ctx context.Context, opts ListOptions) ([]*User, int64, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserByID(ctx context.Context, ID int64) (*User, error)
	CreateUser(ctx context.Context, user *User) error
	UpdateUser(ctx context.Context, user *User, userID int64) error
	DeleteUser(ctx context.Context, userID int64) error
}

type UserService interface {
	Get(ctx context.Context, opts ListOptions) (*ListResult[*User], error)
	Create(ctx context.Context, req UserSaveRequest) error
	Update(ctx context.Context, req UserSaveRequest, userID string) error
	Delete(ctx context.Context, userID string) error
}
