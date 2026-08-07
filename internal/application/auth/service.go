// Package auth
package auth

import (
	"context"
	"strconv"
	"time"

	"horizonx/internal/domain"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	repo         domain.UserRepository
	sessions     domain.SessionStore
	jwtSecret    string
	jwtExpiry    time.Duration
}

func NewService(repo domain.UserRepository, sessions domain.SessionStore, jwtSecret string, jwtExpiry time.Duration) domain.AuthService {
	return &Service{
		repo:      repo,
		sessions:  sessions,
		jwtSecret: jwtSecret,
		jwtExpiry: jwtExpiry,
	}
}

func (s *Service) GetUser(ctx context.Context) (*domain.User, error) {
	userCtx, ok := domain.GetUserContext(ctx)
	if !ok {
		return nil, domain.ErrUnauthorized
	}

	return s.repo.GetByID(ctx, userCtx.ID)
}

func (s *Service) Login(ctx context.Context, req domain.LoginRequest) (*domain.AuthResponse, error) {
	user, err := s.repo.GetByEmail(ctx, req.Email)
	if err != nil {
		return nil, domain.ErrInvalidCredentials
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password))
	if err != nil {
		return nil, domain.ErrInvalidCredentials
	}

	// Create a revocable server-side session. The JWT is stateless; the
	// session ID lets logout / password change / admin kick kill it early.
	sessionID := uuid.NewString()
	now := time.Now()
	sess := &domain.Session{
		ID:        sessionID,
		UserID:    user.ID,
		CreatedAt: now,
		ExpiresAt: now.Add(s.jwtExpiry),
		IP:        domain.ClientIPFromContext(ctx),
		UserAgent: domain.UserAgentFromContext(ctx),
	}
	if err := s.sessions.Create(ctx, sess); err != nil {
		return nil, err
	}

	claims := domain.AuthClaims{
		UserID:    user.ID,
		Role:      user.Role.Name,
		SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(s.jwtExpiry)),
			IssuedAt:  jwt.NewNumericDate(now),
			Subject:   strconv.FormatInt(user.ID, 10),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	tokenString, err := token.SignedString([]byte(s.jwtSecret))
	if err != nil {
		return nil, err
	}

	return &domain.AuthResponse{
		AccessToken: tokenString,
		User:        user,
	}, nil
}

// Logout revokes the current session. The JWT itself remains valid until
// expiry (stateless), but the middleware refuses tokens whose session was
// deleted — so logout now takes effect immediately.
func (s *Service) Logout(ctx context.Context) error {
	userCtx, ok := domain.GetUserContext(ctx)
	if !ok {
		return domain.ErrUnauthorized
	}
	if userCtx.SessionID != "" {
		return s.sessions.Delete(ctx, userCtx.SessionID)
	}
	return nil
}

// RevokeAllSessions kills every session for a user (password change, admin
// kick). The user must log in again everywhere.
func (s *Service) RevokeAllSessions(ctx context.Context, userID int64) error {
	return s.sessions.DeleteAllForUser(ctx, userID)
}
