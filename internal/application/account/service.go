package account

import (
	"context"

	"horizonx/internal/domain"

	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	repo domain.UserRepository
}

func NewService(repo domain.UserRepository) domain.AccountService {
	return &Service{repo: repo}
}

func (s *Service) UpdateProfile(ctx context.Context, req domain.AccountProfileRequest) error {
	userCtx, ok := domain.GetUserContext(ctx)
	if !ok {
		return domain.ErrUnauthorized
	}

	user, err := s.repo.GetByID(ctx, userCtx.ID)
	if err != nil {
		return err
	}

	user.Name = req.Name

	return s.repo.Update(ctx, user, user.ID)
}

func (s *Service) ChangePassword(ctx context.Context, req domain.AccountPasswordRequest) error {
	userCtx, ok := domain.GetUserContext(ctx)
	if !ok {
		return domain.ErrUnauthorized
	}

	user, err := s.repo.GetByID(ctx, userCtx.ID)
	if err != nil {
		return err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.CurrentPassword)); err != nil {
		return domain.ErrInvalidCurrentPassword
	}

	newHashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user.Password = string(newHashedPassword)

	return s.repo.Update(ctx, user, user.ID)
}
