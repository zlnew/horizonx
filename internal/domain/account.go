package domain

import (
	"context"
	"errors"
)

var (
	ErrInvalidCurrentPassword = errors.New("invalid current password")
	ErrSessionNotFound        = errors.New("session not found")
)

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
	// ListSessions returns the caller's registered sessions (this device
	// first via SessionID on the user context, rest newest-first).
	ListSessions(ctx context.Context) ([]*Session, error)
	// RevokeSession terminates one of the caller's sessions. It MUST verify
	// the session belongs to the caller — an authenticated user must not be
	// able to kill another user's session by guessing an ID.
	RevokeSession(ctx context.Context, sessionID string) error
	// RevokeOtherSessions terminates every session except the current one.
	RevokeOtherSessions(ctx context.Context) error
}
