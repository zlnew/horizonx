package account

import (
	"context"

	"horizonx/internal/domain"

	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	repo     domain.UserRepository
	sessions domain.SessionStore
}

func NewService(repo domain.UserRepository, sessions domain.SessionStore) domain.AccountService {
	return &Service{repo: repo, sessions: sessions}
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

	// Password changed — revoke every session so the old token is dead
	// everywhere immediately (the user re-logs in with the new password).
	if err := s.repo.Update(ctx, user, user.ID); err != nil {
		return err
	}
	if s.sessions != nil {
		return s.sessions.DeleteAllForUser(ctx, userCtx.ID)
	}
	return nil
}
