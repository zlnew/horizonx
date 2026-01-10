package domain

import (
	"context"
	"errors"
)

var ErrInvalidCurrentPassword = errors.New("invalid current password")

type AccountProfileRequest struct {
	Name string `json:"name" validate:"required"`
}

type AccountPasswordRequest struct {
	CurrentPassword      string `json:"current_password" validate:"required,min=8"`
	Password             string `json:"password" validate:"required,min=8"`
	PasswordConfirmation string `json:"password_confirmation" validate:"required,min=8,eqfield=Password"`
}

type AccountService interface {
	UpdateProfile(ctx context.Context, req AccountProfileRequest) error
	ChangePassword(ctx context.Context, req AccountPasswordRequest) error
}
