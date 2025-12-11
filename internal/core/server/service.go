// Package server
package server

import (
	"context"

	"horizonx-server/internal/domain"
	"horizonx-server/pkg"
)

type Service struct {
	repo domain.ServerRepository
}

func NewService(repo domain.ServerRepository) domain.ServerService {
	return &Service{repo: repo}
}

func (s *Service) Get(ctx context.Context) ([]domain.Server, error) {
	return s.repo.List(ctx)
}

func (s *Service) Register(ctx context.Context, req domain.ServerSaveRequest) (*domain.Server, string, error) {
	token, err := pkg.GenerateToken()
	if err != nil {
		return nil, "", err
	}

	srv := &domain.Server{
		Name:      req.Name,
		IPAddress: req.IPAddress,
		APIToken:  token,
		IsOnline:  false,
	}

	if err := s.repo.Create(ctx, srv); err != nil {
		return nil, "", err
	}

	return srv, token, nil
}

func (s *Service) Update(ctx context.Context, req domain.ServerSaveRequest, serverID int64) error {
	_, err := s.repo.GetByID(ctx, serverID)
	if err != nil {
		return err
	}

	server := &domain.Server{
		Name:      req.Name,
		IPAddress: req.IPAddress,
	}

	return s.repo.Update(ctx, server, serverID)
}

func (s *Service) Delete(ctx context.Context, serverID int64) error {
	if _, err := s.repo.GetByID(ctx, serverID); err != nil {
		return err
	}

	return s.repo.Delete(ctx, serverID)
}

func (s *Service) AuthorizeAgent(ctx context.Context, token string) (*domain.Server, error) {
	server, err := s.repo.GetByToken(ctx, token)
	if err != nil {
		return nil, err
	}

	return server, nil
}

func (s *Service) UpdateStatus(ctx context.Context, serverID int64, status bool) error {
	return s.repo.UpdateStatus(ctx, serverID, status)
}
