// Package user
package user

import (
	"context"

	"horizonx-server/internal/domain"

	"golang.org/x/crypto/bcrypt"
)

type service struct {
	repo domain.UserRepository
}

func NewService(repo domain.UserRepository) domain.UserService {
	return &service{repo: repo}
}

func (s *service) List(ctx context.Context, opts domain.ListOptions) (*domain.ListResult[*domain.User], error) {
	if opts.IsPaginate {
		if opts.Page <= 0 {
			opts.Page = 1
		}
		if opts.Limit <= 0 {
			opts.Limit = 10
		}
	}

	users, total, err := s.repo.List(ctx, opts)
	if err != nil {
		return nil, err
	}

	res := &domain.ListResult[*domain.User]{
		Data: users,
		Meta: nil,
	}

	if opts.IsPaginate {
		res.Meta = domain.CalculateMeta(total, opts.Page, opts.Limit)
	}

	return res, nil
}

func (s *service) Create(ctx context.Context, req domain.UserSaveRequest) error {
	if _, err := s.repo.GetRoleByID(ctx, req.RoleID); err != nil {
		return err
	}

	if user, _ := s.repo.GetByEmail(ctx, req.Email); user != nil {
		return domain.ErrEmailAlreadyExists
	}

	hashedPwd, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user := &domain.User{
		Name:     req.Name,
		Email:    req.Email,
		Password: string(hashedPwd),
		RoleID:   req.RoleID,
	}

	return s.repo.Create(ctx, user)
}

func (s *service) Update(ctx context.Context, req domain.UserSaveRequest, userID int64) error {
	existingUser, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	if req.RoleID != existingUser.RoleID {
		if _, err := s.repo.GetRoleByID(ctx, req.RoleID); err != nil {
			return err
		}
	}

	if req.Email != existingUser.Email {
		if userCheck, _ := s.repo.GetByEmail(ctx, req.Email); userCheck != nil {
			return domain.ErrEmailAlreadyExists
		}
	}

	passwordToSave := existingUser.Password

	if req.Password != "" {
		hashedPwd, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		passwordToSave = string(hashedPwd)
	}

	user := &domain.User{
		ID:       userID,
		Name:     req.Name,
		Email:    req.Email,
		Password: passwordToSave,
		RoleID:   req.RoleID,
	}

	return s.repo.Update(ctx, user, userID)
}

func (s *service) Delete(ctx context.Context, userID int64) error {
	if _, err := s.repo.GetByID(ctx, userID); err != nil {
		return err
	}

	return s.repo.Delete(ctx, userID)
}
