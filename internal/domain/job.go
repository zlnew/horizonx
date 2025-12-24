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

type JobStatus string

const (
	JobQueued  JobStatus = "queued"
	JobRunning JobStatus = "running"
	JobSuccess JobStatus = "success"
	JobFailed  JobStatus = "failed"
)

type Job struct {
	ID             int64           `json:"id"`
	ServerID       uuid.UUID       `json:"server_id"`
	ApplicationID  *int64          `json:"application_id"`
	DeploymentID   *int64          `json:"deployment_id"`
	JobType        string          `json:"job_type"`
	CommandPayload json.RawMessage `json:"command_payload"`
	Status         JobStatus       `json:"status"`
	OutputLog      *string         `json:"output_log"`
	QueuedAt       *time.Time      `json:"queued_at"`
	StartedAt      *time.Time      `json:"started_at"`
	FinishedAt     *time.Time      `json:"finished_at"`
}

type JobResponse struct {
	ID            int64      `json:"id"`
	ServerID      uuid.UUID  `json:"server_id"`
	ApplicationID *int64     `json:"application_id"`
	DeploymentID  *int64     `json:"deployment_id"`
	JobType       string     `json:"job_type"`
	Status        JobStatus  `json:"status"`
	OutputLog     *string    `json:"output_log"`
	QueuedAt      *time.Time `json:"queued_at"`
	StartedAt     *time.Time `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at"`
}

type JobListOptions struct {
	ListOptions
	ServerID      *uuid.UUID `json:"server_id,omitempty"`
	ApplicationID *int64     `json:"application_id,omitempty"`
	DeploymentID  *int64     `json:"deployment_id,omitempty"`
	JobType       string     `json:"job_type,omitempty"`
	Statuses      []string   `json:"statuses,omitempty"`
}

type JobFinishRequest struct {
	Status    JobStatus `json:"status" validate:"required"`
	OutputLog string    `json:"output_log" validate:"required"`
}

type JobRepository interface {
	List(ctx context.Context, opts JobListOptions) ([]*Job, int64, error)
	GetPending(ctx context.Context) ([]*Job, error)
	GetByID(ctx context.Context, jobID int64) (*Job, error)
	Create(ctx context.Context, j *Job) (*Job, error)
	Delete(ctx context.Context, jobID int64) error
	MarkRunning(ctx context.Context, jobID int64) (*Job, error)
	MarkFinished(ctx context.Context, jobID int64, status JobStatus, outputLog *string) (*Job, error)
}

type JobService interface {
	List(ctx context.Context, opts JobListOptions) (*ListResult[*Job], error)
	GetPending(ctx context.Context) ([]*Job, error)
	GetByID(ctx context.Context, jobID int64) (*Job, error)
	Create(ctx context.Context, j *Job) (*Job, error)
	Delete(ctx context.Context, jobID int64) error
	Start(ctx context.Context, jobID int64) (*Job, error)
	Finish(ctx context.Context, jobID int64, status JobStatus, outputLog *string) (*Job, error)
}
