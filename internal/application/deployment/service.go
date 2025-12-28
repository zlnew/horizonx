// Package deployment
package deployment

import (
	"context"

	"horizonx-server/internal/domain"
	"horizonx-server/internal/event"
)

type Service struct {
	repo   domain.DeploymentRepository
	logSvc domain.LogService
	bus    *event.Bus
}

func NewService(repo domain.DeploymentRepository, logSvc domain.LogService, bus *event.Bus) domain.DeploymentService {
	return &Service{
		repo:   repo,
		logSvc: logSvc,
		bus:    bus,
	}
}

func (s *Service) List(ctx context.Context, opts domain.DeploymentListOptions) (*domain.ListResult[*domain.Deployment], error) {
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

	deployments, total, err := s.repo.List(ctx, opts)
	if err != nil {
		return nil, err
	}

	res := &domain.ListResult[*domain.Deployment]{
		Data: deployments,
		Meta: nil,
	}

	if opts.IsPaginate {
		res.Meta = domain.CalculateMeta(total, opts.Page, opts.Limit)
	}

	return res, nil
}

func (s *Service) GetByID(ctx context.Context, deploymentID int64) (*domain.Deployment, error) {
	deployment, err := s.repo.GetByID(ctx, deploymentID)
	if err != nil {
		return nil, err
	}

	logs, err := s.logSvc.List(ctx, domain.LogListOptions{
		DeploymentID: &deployment.ID,
	})
	if err != nil {
		return nil, err
	}

	if len(logs.Data) > 0 {
		deployment.Logs = make([]domain.Log, 0, len(logs.Data))
		for _, l := range logs.Data {
			if l == nil {
				continue
			}
			deployment.Logs = append(deployment.Logs, *l)
		}
	}

	return deployment, err
}

func (s *Service) Create(ctx context.Context, req domain.DeploymentCreateRequest) (*domain.Deployment, error) {
	deployment := &domain.Deployment{
		ApplicationID: req.ApplicationID,
		Branch:        req.Branch,
		DeployedBy:    req.DeployedBy,
		Status:        domain.DeploymentPending,
	}

	created, err := s.repo.Create(ctx, deployment)
	if err != nil {
		return nil, err
	}

	if s.bus != nil {
		s.bus.Publish("deployment_created", domain.EventDeploymentCreated{
			DeploymentID:  created.ID,
			ApplicationID: created.ApplicationID,
			DeployedBy:    *created.DeployedBy,
			TriggeredAt:   created.TriggeredAt,
		})
	}

	return created, nil
}

func (s *Service) Start(ctx context.Context, deploymentID int64) error {
	d, err := s.repo.Start(ctx, deploymentID)
	if err != nil {
		return err
	}

	if s.bus != nil {
		s.bus.Publish("deployment_started", domain.EventDeploymentStarted{
			DeploymentID:  d.ID,
			ApplicationID: d.ApplicationID,
			StartedAt:     *d.StartedAt,
		})
	}

	return nil
}

func (s *Service) Finish(ctx context.Context, deploymentID int64) error {
	d, err := s.repo.Finish(ctx, deploymentID)
	if err != nil {
		return err
	}

	if s.bus != nil {
		s.bus.Publish("deployment_finished", domain.EventDeploymentFinished{
			DeploymentID:  d.ID,
			ApplicationID: d.ApplicationID,
			Status:        d.Status,
			FinishedAt:    *d.FinishedAt,
		})
	}

	return nil
}

func (s *Service) UpdateStatus(ctx context.Context, deploymentID int64, status domain.DeploymentStatus) error {
	d, err := s.repo.UpdateStatus(ctx, deploymentID, status)
	if err != nil {
		return err
	}

	if s.bus != nil {
		s.bus.Publish("deployment_status_changed", domain.EventDeploymentStatusChanged{
			DeploymentID:  d.ID,
			ApplicationID: d.ApplicationID,
			Status:        d.Status,
		})
	}

	return nil
}

func (s *Service) UpdateCommitInfo(ctx context.Context, deploymentID int64, commitHash string, commitMessage string) error {
	d, err := s.repo.UpdateCommitInfo(ctx, deploymentID, commitHash, commitMessage)
	if err != nil {
		return err
	}

	if s.bus != nil {
		s.bus.Publish("deployment_commit_info_received", domain.EventDeploymentCommitInfoReceived{
			DeploymentID:  d.ID,
			ApplicationID: d.ApplicationID,
			CommitHash:    *d.CommitHash,
			CommitMessage: *d.CommitMessage,
		})
	}

	return nil
}
