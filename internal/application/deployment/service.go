// Package deployment
package deployment

import (
	"context"

	"horizonx-server/internal/domain"
	"horizonx-server/internal/event"
)

type Service struct {
	repo domain.DeploymentRepository
	bus  *event.Bus
}

func NewService(repo domain.DeploymentRepository, bus *event.Bus) domain.DeploymentService {
	return &Service{
		repo: repo,
		bus:  bus,
	}
}

func (s *Service) List(ctx context.Context, appID int64, limit int) ([]domain.Deployment, error) {
	return s.repo.List(ctx, appID, limit)
}

func (s *Service) GetByID(ctx context.Context, deploymentID int64) (*domain.Deployment, error) {
	return s.repo.GetByID(ctx, deploymentID)
}

func (s *Service) GetLatest(ctx context.Context, appID int64) (*domain.Deployment, error) {
	return s.repo.GetLatest(ctx, appID)
}

func (s *Service) Create(ctx context.Context, req domain.DeploymentCreateRequest) (*domain.Deployment, error) {
	deployment := &domain.Deployment{
		ApplicationID: req.ApplicationID,
		JobID:         req.JobID,
		Branch:        req.Branch,
		Environment:   req.Environment,
		DeployedBy:    req.DeployedBy,
		Status:        domain.DeploymentPending,
	}

	created, err := s.repo.Create(ctx, deployment)
	if err != nil {
		return nil, err
	}

	// Publish event
	if s.bus != nil {
		s.bus.Publish("deployment_created", domain.EventDeploymentCreated{
			Deployment: created,
		})

		s.bus.Publish("deployment_status_changed", domain.EventDeploymentStatusChanged{
			DeploymentID:  created.ID,
			ApplicationID: created.ApplicationID,
			Status:        created.Status,
		})
	}

	return created, nil
}
