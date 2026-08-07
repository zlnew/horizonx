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

func TestAuthService_Login_CreatesSession(t *testing.T) {
	mockRepo := mocks.NewMockUserRepository(t)

	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	mockUser := &domain.User{
		ID:       1,
		Name:     "Admin",
		Email:    "admin@horizonx.local",
		Password: string(hashedPassword),
		RoleID:   1,
		Role:     &domain.Role{ID: 1, Name: "admin"},
	}

	mockRepo.EXPECT().
		GetByEmail(mock.Anything, "admin@horizonx.local").
		Return(mockUser, nil)

	store := &fakeSessionStore{}
	svc := auth.NewService(mockRepo, store, "secret", time.Hour)

	ctx := domain.SetClientIP(context.Background(), "203.0.113.7")
	ctx = domain.SetUserAgent(ctx, "test-agent")

	res, err := svc.Login(ctx, domain.LoginRequest{Email: "admin@horizonx.local", Password: "password123"})
	assert.NoError(t, err)
	assert.NotEmpty(t, res.AccessToken)
	assert.Len(t, store.created, 1)
	assert.Equal(t, int64(1), store.created[0].UserID)
	assert.Equal(t, "203.0.113.7", store.created[0].IP)
	assert.Equal(t, "test-agent", store.created[0].UserAgent)
	assert.NotEmpty(t, store.created[0].ID, "session must have an ID embedded in the token")
}

func TestAuthService_Logout_DeletesSession(t *testing.T) {
	store := &fakeSessionStore{}
	svc := auth.NewService(nil, store, "secret", time.Hour)

	ctx := domain.SetUserContext(context.Background(), domain.UserContext{ID: 1, SessionID: "sess-1"})
	err := svc.Logout(ctx)
	assert.NoError(t, err)
	assert.Equal(t, []string{"sess-1"}, store.deleted)
}

func TestAuthService_RevokeAllSessions(t *testing.T) {
	store := &fakeSessionStore{}
	svc := auth.NewService(nil, store, "secret", time.Hour)

	err := svc.RevokeAllSessions(context.Background(), 42)
	assert.NoError(t, err)
	assert.True(t, store.deletedAllFor[42])
}
