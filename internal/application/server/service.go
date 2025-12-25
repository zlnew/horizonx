// Package server
package server

import (
	"context"

	"horizonx-server/internal/domain"
	"horizonx-server/internal/event"
	"horizonx-server/internal/pkg"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	repo domain.ServerRepository
	bus  *event.Bus
}

func NewService(repo domain.ServerRepository, bus *event.Bus) domain.ServerService {
	return &Service{
		repo: repo,
		bus:  bus,
	}
}

func (s *Service) List(ctx context.Context, opts domain.ServerListOptions) (*domain.ListResult[*domain.Server], error) {
	if opts.IsPaginate {
		if opts.Page <= 0 {
			opts.Page = 1
		}
		if opts.Limit <= 0 {
			opts.Limit = 10
		}
	} else {
		if opts.Limit <= 0 {
			opts.Limit = 1000
		}
	}

	servers, total, err := s.repo.List(ctx, opts)
	if err != nil {
		return nil, err
	}

	res := &domain.ListResult[*domain.Server]{
		Data: servers,
		Meta: nil,
	}

	if opts.IsPaginate {
		res.Meta = domain.CalculateMeta(total, opts.Page, opts.Limit)
	}

	return res, nil
}

func (s *Service) GetByID(ctx context.Context, serverID uuid.UUID) (*domain.Server, error) {
	return s.repo.GetByID(ctx, serverID)
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
	if err := s.repo.UpdateStatus(ctx, serverID, status); err != nil {
		return err
	}

	s.bus.Publish("server_status_changed", domain.EventServerStatusChanged{
		ServerID: serverID,
		IsOnline: status,
	})

	return nil
}
