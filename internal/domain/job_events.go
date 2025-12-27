package domain

import (
	"time"

	"github.com/google/uuid"
)

type EventJobCreated struct {
	JobID         int64     `json:"job_id"`
	TraceID       uuid.UUID `json:"trace_id"`
	ServerID      uuid.UUID `json:"server_id"`
	ApplicationID *int64    `json:"application_id"`
	DeploymentID  *int64    `json:"deployment_id"`
	Type          JobType   `json:"type"`
}

type EventJobStarted struct {
	JobID         int64     `json:"job_id"`
	TraceID       uuid.UUID `json:"trace_id"`
	ServerID      uuid.UUID `json:"server_id"`
	ApplicationID *int64    `json:"application_id"`
	DeploymentID  *int64    `json:"deployment_id"`
	Type          JobType   `json:"type"`
}

type EventJobFinished struct {
	JobID         int64     `json:"job_id"`
	TraceID       uuid.UUID `json:"trace_id"`
	ServerID      uuid.UUID `json:"server_id"`
	ApplicationID *int64    `json:"application_id"`
	DeploymentID  *int64    `json:"deployment_id"`
	Type          JobType   `json:"type"`
	Status        JobStatus `json:"status"`
}

type EventJobStatusChanged struct {
	JobID   int64     `json:"job_id"`
	TraceID uuid.UUID `json:"trace_id"`
	Status  JobStatus `json:"status"`
}

type EventLogEmitted struct {
	Timestamp time.Time
	Level     LogLevel
	Source    LogSource
	Action    LogAction
	Message   string
	Context   *LogContext
}

type EventCommitInfoEmitted struct {
	DeploymentID int64
	Hash         string
	Message      string
}
