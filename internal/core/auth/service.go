package auth

import (
	"context"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type service struct {
	repo        UserRepository
	jwtSecret   []byte
	tokenExpiry time.Duration
}

func NewService(repo UserRepository, secret string, expiry time.Duration) AuthService {
	return &service{
		repo:        repo,
		jwtSecret:   []byte(secret),
		tokenExpiry: expiry,
	}
}

func (s *service) Register(ctx context.Context, req RegisterRequest) error {
	hashedPwd, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user := &User{
		Email:    req.Email,
		Password: string(hashedPwd),
	}

	return s.repo.CreateUser(ctx, user)
}

func (s *service) Login(ctx context.Context, req LoginRequest) (*AuthResponse, error) {
	user, err := s.repo.GetUserByEmail(ctx, req.Email)
	if err != nil {
		return nil, errors.New("invalid email or password")
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password))
	if err != nil {
		return nil, errors.New("invalid email or password")
	}

	claims := jwt.MapClaims{
		"sub":   user.ID,
		"email": user.Email,
		"exp":   time.Now().Add(s.tokenExpiry).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{
		AccessToken: tokenString,
		User:        user,
	}, nil
}
