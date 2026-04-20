package domain

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidCredentials    = errors.New("invalid credentials")
	ErrUnauthorized          = errors.New("unauthorized")
	ErrYouDontHavePermission = errors.New("you don't have permission")
)

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
}

type AuthResponse struct {
	User        *User  `json:"user"`
	AccessToken string `json:"access_token"`
}

type AuthService interface {
	GetUser(ctx context.Context) (*User, error)
	Login(ctx context.Context, req LoginRequest) (*AuthResponse, error)
}

type AuthClaims struct {
	UserID int64     `json:"sub"`
	Role   RoleConst `json:"role"`
	jwt.RegisteredClaims
}

type contextKey struct{}

var UserContextKey = contextKey{}

func SetUserContext(ctx context.Context, u UserContext) context.Context {
	return context.WithValue(ctx, UserContextKey, u)
}

func GetUserContext(ctx context.Context) (UserContext, bool) {
	u, ok := ctx.Value(UserContextKey).(UserContext)
	return u, ok
}

func GenerateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "hzx_" + hex.EncodeToString(bytes), nil
}

func ValidateToken(tokenString, secret string) (*AuthClaims, error) {
	claims := &AuthClaims{}

	token, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		func(token *jwt.Token) (any, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(secret), nil
		},
	)
	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, errors.New("invalid token")
	}

	return claims, nil
}
