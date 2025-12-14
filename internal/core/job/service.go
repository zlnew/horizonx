// Package job
package job

import (
	"context"

	"horizonx-server/internal/core/event"
	"horizonx-server/internal/domain"
)

type JobService struct {
	repo   domain.JobRepository
	events *event.Bus
}

func NewService(repo domain.JobRepository, events *event.Bus) domain.JobService {
	return &JobService{
		repo:   repo,
		events: events,
	}
}

func (s *JobService) Get(ctx context.Context) ([]domain.Job, error) {
	return s.repo.List(ctx)
}

func (s *JobService) Create(ctx context.Context, j *domain.Job) (*domain.Job, error) {
	job, err := s.repo.Create(ctx, j)
	if err != nil {
		return nil, err
	}

	if s.events != nil {
		s.events.Publish(domain.EventJobCreated{
			JobID:    job.ID,
			ServerID: job.ServerID,
			JobType:  job.JobType,
		})
	}

	return job, nil
}

func (s *JobService) Delete(ctx context.Context, jobID int64) error {
	return s.repo.Delete(ctx, jobID)
}

func (s *JobService) Start(ctx context.Context, jobID int64) (*domain.Job, error) {
	return s.repo.MarkRunning(ctx, jobID)
}

func (s *JobService) Finish(ctx context.Context, jobID int64, status domain.JobStatus, result *string) (*domain.Job, error) {
	job, err := s.repo.MarkFinished(ctx, jobID, status, result)
	if err != nil {
		return nil, err
	}

	if s.events != nil {
		s.events.Publish(domain.EventJobFinished{
			JobID:    job.ID,
			ServerID: job.ServerID,
			JobType:  job.JobType,
			Result:   job.OutputLog,
		})
	}

	return job, err
}
