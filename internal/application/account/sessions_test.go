package account_test

import (
	"context"
	"testing"
	"time"

	"horizonx/internal/application/account"
	"horizonx/internal/domain"
	"horizonx/internal/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestAccountService_ListSessions_CurrentFirst(t *testing.T) {
	mockUserRepo := mocks.NewMockUserRepository(t)

	now := time.Now()
	sessions := []*domain.Session{
		{ID: "oldest", UserID: 1, CreatedAt: now.Add(-2 * time.Hour)},
		{ID: "current", UserID: 1, CreatedAt: now.Add(-1 * time.Hour)},
		{ID: "newest", UserID: 1, CreatedAt: now.Add(-30 * time.Minute)},
	}

	svc := account.NewService(mockUserRepo, newFakeSessionStore(sessions...))
	ctx := domain.SetUserContext(context.Background(), domain.UserContext{ID: 1, SessionID: "current"})

	got, err := svc.ListSessions(ctx)

	assert.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, "current", got[0].ID)
	assert.Equal(t, "newest", got[1].ID)
	assert.Equal(t, "oldest", got[2].ID)
}

func TestAccountService_ListSessions_Unauthorized(t *testing.T) {
	mockUserRepo := mocks.NewMockUserRepository(t)

	svc := account.NewService(mockUserRepo, newFakeSessionStore())
	_, err := svc.ListSessions(context.Background())

	assert.ErrorIs(t, err, domain.ErrUnauthorized)
}

func TestAccountService_RevokeSession_OwnSession(t *testing.T) {
	mockUserRepo := mocks.NewMockUserRepository(t)

	sessions := []*domain.Session{
		{ID: "mine", UserID: 1},
		{ID: "other-user", UserID: 2},
	}

	svc := account.NewService(mockUserRepo, newFakeSessionStore(sessions...))
	ctx := domain.SetUserContext(context.Background(), domain.UserContext{ID: 1})

	err := svc.RevokeSession(ctx, "mine")

	assert.NoError(t, err)
}

// The security-critical case: a user must NOT be able to terminate another
// user's session by guessing its ID.
func TestAccountService_RevokeSession_NotOwnSession_Forbidden(t *testing.T) {
	mockUserRepo := mocks.NewMockUserRepository(t)

	sessions := []*domain.Session{
		{ID: "mine", UserID: 1},
		{ID: "other-user", UserID: 2},
	}

	svc := account.NewService(mockUserRepo, newFakeSessionStore(sessions...))
	ctx := domain.SetUserContext(context.Background(), domain.UserContext{ID: 1})

	err := svc.RevokeSession(ctx, "other-user")

	assert.ErrorIs(t, err, domain.ErrSessionNotFound)
}

func TestAccountService_RevokeSession_NotFound(t *testing.T) {
	mockUserRepo := mocks.NewMockUserRepository(t)

	svc := account.NewService(mockUserRepo, newFakeSessionStore())
	ctx := domain.SetUserContext(context.Background(), domain.UserContext{ID: 1})

	err := svc.RevokeSession(ctx, "ghost")

	assert.ErrorIs(t, err, domain.ErrSessionNotFound)
}

func TestAccountService_RevokeOtherSessions_KeepsCurrent(t *testing.T) {
	mockUserRepo := mocks.NewMockUserRepository(t)

	sessions := []*domain.Session{
		{ID: "current", UserID: 1},
		{ID: "phone", UserID: 1},
		{ID: "laptop", UserID: 1},
	}

	fake := newFakeSessionStore(sessions...)
	svc := account.NewService(mockUserRepo, fake)
	ctx := domain.SetUserContext(context.Background(), domain.UserContext{ID: 1, SessionID: "current"})

	err := svc.RevokeOtherSessions(ctx)

	assert.NoError(t, err)
	assert.Contains(t, fake.sessions, "current")
	assert.NotContains(t, fake.sessions, "phone")
	assert.NotContains(t, fake.sessions, "laptop")
}

var _ = mock.Anything
