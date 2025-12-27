// Package job
package job

import (
	"context"

	"horizonx-server/internal/domain"
	"horizonx-server/internal/event"
)

type JobService struct {
	repo    domain.JobRepository
	logRepo domain.LogRepository
	bus     *event.Bus
}

func NewService(repo domain.JobRepository, logRepo domain.LogRepository, events *event.Bus) domain.JobService {
	return &JobService{
		repo:    repo,
		logRepo: logRepo,
		bus:     events,
	}
}

func (s *JobService) List(ctx context.Context, opts domain.JobListOptions) (*domain.ListResult[*domain.Job], error) {
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

	jobs, total, err := s.repo.List(ctx, opts)
	if err != nil {
		return nil, err
	}

	res := &domain.ListResult[*domain.Job]{
		Data: jobs,
		Meta: nil,
	}

	if opts.IsPaginate {
		res.Meta = domain.CalculateMeta(total, opts.Page, opts.Limit)
	}

	return res, nil
}

func (s *JobService) GetPending(ctx context.Context) ([]*domain.Job, error) {
	return s.repo.GetPending(ctx)
}

func (s *JobService) GetByID(ctx context.Context, jobID int64) (*domain.Job, error) {
	job, err := s.repo.GetByID(ctx, jobID)
	if err != nil {
		return nil, err
	}

	logs, _, err := s.logRepo.List(ctx, domain.LogListOptions{
		ListOptions: domain.ListOptions{Limit: 1000},
		JobID:       &job.ID,
	})
	if err != nil {
		return nil, err
	}

	if len(logs) > 0 {
		job.Logs = make([]domain.Log, 0, len(logs))
		for _, l := range logs {
			if l == nil {
				continue
			}
			job.Logs = append(job.Logs, *l)
		}
	}

	return job, err
}

func (s *JobService) Create(ctx context.Context, j *domain.Job) (*domain.Job, error) {
	job, err := s.repo.Create(ctx, j)
	if err != nil {
		return nil, err
	}

	if s.bus != nil {
		s.bus.Publish("job_created", domain.EventJobCreated{
			JobID:         job.ID,
			TraceID:       job.TraceID,
			ServerID:      job.ServerID,
			ApplicationID: job.ApplicationID,
			DeploymentID:  job.DeploymentID,
			Type:          job.Type,
		})

		s.bus.Publish("job_status_changed", domain.EventJobStatusChanged{
			JobID:   job.ID,
			TraceID: job.TraceID,
			Status:  job.Status,
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
			TraceID:       job.TraceID,
			ServerID:      job.ServerID,
			ApplicationID: job.ApplicationID,
			DeploymentID:  job.DeploymentID,
			Type:          job.Type,
		})

		s.bus.Publish("job_status_changed", domain.EventJobStatusChanged{
			JobID:   job.ID,
			TraceID: job.TraceID,
			Status:  job.Status,
		})
	}

	return job, nil
}

func (s *JobService) Finish(ctx context.Context, jobID int64, status domain.JobStatus) (*domain.Job, error) {
	job, err := s.repo.MarkFinished(ctx, jobID, status)
	if err != nil {
		return nil, err
	}

	if s.bus != nil {
		s.bus.Publish("job_finished", domain.EventJobFinished{
			JobID:         job.ID,
			TraceID:       job.TraceID,
			ServerID:      job.ServerID,
			ApplicationID: job.ApplicationID,
			DeploymentID:  job.DeploymentID,
			Type:          job.Type,
			Status:        status,
		})

		s.bus.Publish("job_status_changed", domain.EventJobStatusChanged{
			JobID:   job.ID,
			TraceID: job.TraceID,
			Status:  job.Status,
		})
	}

	return job, err
}
