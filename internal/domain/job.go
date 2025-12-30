package domain

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrJobNotFound     = errors.New("job not found")
	ErrInvalidJobState = errors.New("invalid job state")
)

type (
	JobType   string
	JobStatus string
)

const (
	JobTypeAppDeploy      JobType = "app_deploy"
	JobTypeAppStart       JobType = "app_start"
	JobTypeAppStop        JobType = "app_stop"
	JobTypeAppRestart     JobType = "app_restart"
	JobTypeAppHealthCheck JobType = "app_health_check"
	JobTypeMetricsCollect JobType = "metrics_collect"
)

const (
	JobQueued  JobStatus = "queued"
	JobRunning JobStatus = "running"
	JobSuccess JobStatus = "success"
	JobFailed  JobStatus = "failed"
	JobExpired JobStatus = "expired"
)

type Job struct {
	ID            int64           `json:"id"`
	TraceID       uuid.UUID       `json:"trace_id"`
	ServerID      uuid.UUID       `json:"server_id"`
	ApplicationID *int64          `json:"application_id"`
	DeploymentID  *int64          `json:"deployment_id"`
	Type          JobType         `json:"type"`
	Payload       json.RawMessage `json:"payload"`
	Status        JobStatus       `json:"status"`
	QueuedAt      *time.Time      `json:"queued_at"`
	StartedAt     *time.Time      `json:"started_at"`
	FinishedAt    *time.Time      `json:"finished_at"`
	ExpiredAt     *time.Time      `json:"expired_at"`

	Logs []Log `json:"logs,omitempty"`
}

type JobListOptions struct {
	ListOptions
	TraceID       *uuid.UUID `json:"trace_id,omitempty"`
	ServerID      *uuid.UUID `json:"server_id,omitempty"`
	ApplicationID *int64     `json:"application_id,omitempty"`
	DeploymentID  *int64     `json:"deployment_id,omitempty"`
	Type          string     `json:"type,omitempty"`
	Statuses      []string   `json:"statuses,omitempty"`
}

type JobFinishRequest struct {
	Status JobStatus `json:"status"`
}

type JobRepository interface {
	List(ctx context.Context, opts JobListOptions) ([]*Job, int64, error)
	GetPending(ctx context.Context, serverID uuid.UUID) ([]*Job, error)
	GetByID(ctx context.Context, jobID int64) (*Job, error)
	Create(ctx context.Context, j *Job) (*Job, error)
	Delete(ctx context.Context, jobID int64) error
	Retry(ctx context.Context, jobID int64, j *Job) (*Job, error)
	MarkRunning(ctx context.Context, jobID int64) (*Job, error)
	MarkFinished(ctx context.Context, jobID int64, status JobStatus) (*Job, error)
}

type JobService interface {
	List(ctx context.Context, opts JobListOptions) (*ListResult[*Job], error)
	GetPending(ctx context.Context, serverID uuid.UUID) ([]*Job, error)
	GetByID(ctx context.Context, jobID int64) (*Job, error)
	Create(ctx context.Context, j *Job) (*Job, error)
	Delete(ctx context.Context, jobID int64) error
	Retry(ctx context.Context, jobID int64, j *Job) (*Job, error)
	Start(ctx context.Context, jobID int64) (*Job, error)
	Finish(ctx context.Context, jobID int64, status JobStatus) (*Job, error)
}
