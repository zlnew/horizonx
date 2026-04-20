// Package auth
package auth

import (
	"context"
	"strconv"
	"time"

	"horizonx/internal/domain"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	repo      domain.UserRepository
	jwtSecret string
	jwtExpiry time.Duration
}

func NewService(repo domain.UserRepository, jwtSecret string, jwtExpiry time.Duration) domain.AuthService {
	return &Service{
		repo:      repo,
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

	claims := domain.AuthClaims{
		UserID: user.ID,
		Role:   user.Role.Name,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.jwtExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
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
