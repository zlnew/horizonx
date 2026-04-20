package auth_test

import (
	"context"
	"testing"
	"time"

	"horizonx/internal/application/auth"
	"horizonx/internal/domain"
	"horizonx/internal/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/bcrypt"
)

func TestAuthService_GetUser_Success(t *testing.T) {
	mockRepo := mocks.NewMockUserRepository(t)

	mockUser := &domain.User{
		ID:    1,
		Name:  "Admin",
		Email: "admin@horizonx.local",
	}

	mockRepo.EXPECT().
		GetByID(mock.Anything, int64(1)).
		Return(mockUser, nil)

	ctx := domain.SetUserContext(context.Background(), domain.UserContext{ID: 1})

	svc := auth.NewService(mockRepo, "secret", time.Hour)
	realUser, err := svc.GetUser(ctx)

	assert.NoError(t, err)
	assert.Equal(t, mockUser, realUser)
}

func TestAuthService_GetUser_Unauthorized(t *testing.T) {
	mockRepo := mocks.NewMockUserRepository(t)

	svc := auth.NewService(mockRepo, "secret", time.Hour)
	realUser, err := svc.GetUser(context.Background())

	assert.ErrorIs(t, err, domain.ErrUnauthorized)
	assert.Nil(t, realUser)
}

func TestAuthService_Login_Success(t *testing.T) {
	mockRepo := mocks.NewMockUserRepository(t)

	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)

	mockRole := &domain.Role{
		ID:   1,
		Name: "admin",
	}

	mockUser := &domain.User{
		ID:       1,
		Name:     "Admin",
		Email:    "admin@horizonx.local",
		Password: string(hashedPassword),
		RoleID:   mockRole.ID,
		Role:     mockRole,
	}

	mockRepo.EXPECT().
		GetByEmail(mock.Anything, "admin@horizonx.local").
		Return(mockUser, nil)

	svc := auth.NewService(mockRepo, "secret", time.Hour)
	res, err := svc.Login(context.Background(), domain.LoginRequest{
		Email:    "admin@horizonx.local",
		Password: "password",
	})

	assert.NoError(t, err)
	assert.NotNil(t, res)
	assert.NotEmpty(t, res.AccessToken)
	assert.Equal(t, mockUser, res.User)
}

func TestAuthService_Login_InvalidCredentials(t *testing.T) {
	mockRepo := mocks.NewMockUserRepository(t)

	mockRepo.EXPECT().
		GetByEmail(mock.Anything, "ghost@horizonx.local").
		Return(nil, domain.ErrUserNotFound)

	svc := auth.NewService(mockRepo, "secret", time.Hour)
	res, err := svc.Login(context.Background(), domain.LoginRequest{
		Email:    "ghost@horizonx.local",
		Password: "password",
	})

	assert.ErrorIs(t, err, domain.ErrInvalidCredentials)
	assert.Nil(t, res)
}

func TestAuthService_Login_WrongPassword(t *testing.T) {
	mockRepo := mocks.NewMockUserRepository(t)

	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)

	mockRole := &domain.Role{
		ID:   1,
		Name: "admin",
	}

	mockUser := &domain.User{
		ID:       1,
		Name:     "Admin",
		Email:    "admin@horizonx.local",
		Password: string(hashedPassword),
		RoleID:   mockRole.ID,
		Role:     mockRole,
	}

	mockRepo.EXPECT().
		GetByEmail(mock.Anything, "admin@horizonx.local").
		Return(mockUser, nil)

	svc := auth.NewService(mockRepo, "secret", time.Hour)
	res, err := svc.Login(context.Background(), domain.LoginRequest{
		Email:    "admin@horizonx.local",
		Password: "wrong-password",
	})

	assert.ErrorIs(t, err, domain.ErrInvalidCredentials)
	assert.Nil(t, res)
}
