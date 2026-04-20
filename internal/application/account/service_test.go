package account_test

import (
	"context"
	"testing"

	"horizonx/internal/application/account"
	"horizonx/internal/domain"
	"horizonx/internal/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestAccountService_UpdateProfile_Success(t *testing.T) {
	mockUserRepo := mocks.NewMockUserRepository(t)

	mockUser := &domain.User{
		ID:   1,
		Name: "Admin",
	}

	mockUserRepo.EXPECT().
		GetByID(mock.Anything, int64(1)).
		Return(mockUser, nil)

	mockUserRepo.EXPECT().
		Update(mock.Anything, mock.MatchedBy(func(u *domain.User) bool {
			return u.Name == "New Name" && u.ID == int64(1)
		}), int64(1)).
		Return(nil)

	ctx := domain.SetUserContext(context.Background(), domain.UserContext{ID: 1})

	svc := account.NewService(mockUserRepo)
	err := svc.UpdateProfile(ctx, domain.AccountProfileRequest{Name: "New Name"})

	assert.NoError(t, err)
}

func TestAccountService_UpdateProfile_Unauthorized(t *testing.T) {
	mockUserRepo := mocks.NewMockUserRepository(t)

	svc := account.NewService(mockUserRepo)
	err := svc.UpdateProfile(context.Background(), domain.AccountProfileRequest{Name: "New Name"})

	assert.ErrorIs(t, err, domain.ErrUnauthorized)
}

func TestAccountService_UpdateProfile_UserNotFound(t *testing.T) {
	mockUserRepo := mocks.NewMockUserRepository(t)

	mockUserRepo.EXPECT().
		GetByID(mock.Anything, int64(1)).
		Return(nil, domain.ErrUserNotFound)

	ctx := domain.SetUserContext(context.Background(), domain.UserContext{ID: 1})

	svc := account.NewService(mockUserRepo)
	err := svc.UpdateProfile(ctx, domain.AccountProfileRequest{Name: "New Name"})

	assert.ErrorIs(t, err, domain.ErrUserNotFound)
}
