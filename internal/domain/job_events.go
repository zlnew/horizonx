package domain

import "github.com/google/uuid"

type EventJobCreated struct {
	JobID         int64
	ServerID      uuid.UUID
	ApplicationID *int64
	DeploymentID  *int64
	JobType       string
}

type EventJobStarted struct {
	JobID         int64
	ServerID      uuid.UUID
	ApplicationID *int64
	DeploymentID  *int64
	JobType       string
}

type EventJobFinished struct {
	JobID         int64
	ServerID      uuid.UUID
	ApplicationID *int64
	DeploymentID  *int64
	JobType       string
	Status        JobStatus
	OutputLog     *string
}
