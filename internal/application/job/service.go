// Package job
package job

import (
	"context"

	"horizonx-server/internal/domain"
	"horizonx-server/internal/event"
)

type JobService struct {
	repo domain.JobRepository
	bus  *event.Bus
}

func NewService(repo domain.JobRepository, events *event.Bus) domain.JobService {
	return &JobService{
		repo: repo,
		bus:  events,
	}
}

func (s *JobService) Get(ctx context.Context) ([]domain.Job, error) {
	return s.repo.List(ctx)
}

func (s *JobService) GetByID(ctx context.Context, jobID int64) (*domain.Job, error) {
	return s.repo.GetByID(ctx, jobID)
}

func (s *JobService) Create(ctx context.Context, j *domain.Job) (*domain.Job, error) {
	job, err := s.repo.Create(ctx, j)
	if err != nil {
		return nil, err
	}

	if s.bus != nil {
		s.bus.Publish("job_created", domain.EventJobCreated{
			JobID:         job.ID,
			ServerID:      job.ServerID,
			ApplicationID: job.ApplicationID,
			DeploymentID:  job.DeploymentID,
			JobType:       job.JobType,
		})
	}

	return job, nil
}

func (s *JobService) Delete(ctx context.Context, jobID int64) error {
	return s.repo.Delete(ctx, jobID)
}

func (s *JobService) Start(ctx context.Context, jobID int64) (*domain.Job, error) {
	job, err := s.repo.MarkRunning(ctx, jobID)
	if err != nil {
		return nil, err
	}

	if s.bus != nil {
		s.bus.Publish("job_started", domain.EventJobStarted{
			JobID:         job.ID,
			ServerID:      job.ServerID,
			ApplicationID: job.ApplicationID,
			DeploymentID:  job.DeploymentID,
			JobType:       job.JobType,
		})
	}

	return job, nil
}

func (s *JobService) Finish(ctx context.Context, jobID int64, status domain.JobStatus, result *string) (*domain.Job, error) {
	job, err := s.repo.MarkFinished(ctx, jobID, status, result)
	if err != nil {
		return nil, err
	}

	if s.bus != nil {
		s.bus.Publish("job_finished", domain.EventJobFinished{
			JobID:         job.ID,
			ServerID:      job.ServerID,
			ApplicationID: job.ApplicationID,
			DeploymentID:  job.DeploymentID,
			JobType:       job.JobType,
			Status:        status,
			OutputLog:     result,
		})
	}

	return job, err
}
