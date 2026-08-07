package account_test

import (
	"context"
	"testing"

	"horizonx/internal/application/account"
	"horizonx/internal/domain"
	"horizonx/internal/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/bcrypt"
)

func TestAccountService_ChangePassword_RevokesAllSessions(t *testing.T) {
	mockUserRepo := mocks.NewMockUserRepository(t)

	oldHashed, _ := bcrypt.GenerateFromPassword([]byte("oldpassword"), bcrypt.MinCost)
	mockUser := &domain.User{
		ID:       1,
		Name:     "Admin",
		Email:    "admin@horizonx.local",
		Password: string(oldHashed),
	}

	mockUserRepo.EXPECT().
		GetByID(mock.Anything, int64(1)).
		Return(mockUser, nil)
	mockUserRepo.EXPECT().
		Update(mock.Anything, mock.MatchedBy(func(u *domain.User) bool {
			return u.ID == int64(1) && u.Password != string(oldHashed)
		}), int64(1)).
		Return(nil)

	store := &fakeSessionStore{}
	svc := account.NewService(mockUserRepo, store)

	ctx := domain.SetUserContext(context.Background(), domain.UserContext{ID: 1})
	err := svc.ChangePassword(ctx, domain.AccountPasswordRequest{
		CurrentPassword:      "oldpassword",
		Password:             "newpassword123",
		PasswordConfirmation: "newpassword123",
	})

	assert.NoError(t, err)
	assert.True(t, store.deletedAllFor[1], "password change must revoke ALL sessions")
}
