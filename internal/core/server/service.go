// Package server
package server

import (
	"context"

	"horizonx-server/internal/domain"
	"horizonx-server/pkg"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
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

	hashedToken, err := bcrypt.GenerateFromPassword([]byte(token), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", err
	}

	data := &domain.Server{
		Name:      req.Name,
		IPAddress: req.IPAddress,
		APIToken:  string(hashedToken),
		IsOnline:  false,
	}

	srv, err := s.repo.Create(ctx, data)
	if err != nil {
		return nil, "", err
	}

	return srv, token, nil
}

func (s *Service) Update(ctx context.Context, req domain.ServerSaveRequest, serverID uuid.UUID) error {
	_, err := s.repo.GetByID(ctx, serverID)
	if err != nil {
		return err
	}

	data := &domain.Server{
		Name:      req.Name,
		IPAddress: req.IPAddress,
	}

	return s.repo.Update(ctx, data, serverID)
}

func (s *Service) Delete(ctx context.Context, serverID uuid.UUID) error {
	if _, err := s.repo.GetByID(ctx, serverID); err != nil {
		return err
	}

	return s.repo.Delete(ctx, serverID)
}

func (s *Service) AuthorizeAgent(ctx context.Context, serverID uuid.UUID, secret string) (*domain.Server, error) {
	server, err := s.repo.GetByID(ctx, serverID)
	if err != nil {
		return nil, domain.ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword(
		[]byte(server.APIToken),
		[]byte(secret),
	); err != nil {
		return nil, domain.ErrInvalidCredentials
	}

	return server, nil
}

func (s *Service) UpdateStatus(ctx context.Context, serverID uuid.UUID, status bool) error {
	return s.repo.UpdateStatus(ctx, serverID, status)
}
