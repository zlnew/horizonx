package domain

import "time"

type EventDeploymentCreated struct {
	DeploymentID  int64     `json:"deployment_id"`
	ApplicationID int64     `json:"application_id"`
	DeployedBy    int64     `json:"deployed_by"`
	TriggeredAt   time.Time `json:"triggered_at"`
}

type EventDeploymentStarted struct {
	DeploymentID  int64     `json:"deployment_id"`
	ApplicationID int64     `json:"application_id"`
	StartedAt     time.Time `json:"started_at"`
}

type EventDeploymentFinished struct {
	DeploymentID  int64            `json:"deployment_id"`
	ApplicationID int64            `json:"application_id"`
	Status        DeploymentStatus `json:"status"`
	FinishedAt    time.Time        `json:"finished_at"`
}

type EventDeploymentStatusChanged struct {
	DeploymentID  int64            `json:"deployment_id"`
	ApplicationID int64            `json:"application_id"`
	Status        DeploymentStatus `json:"status"`
}

type EventDeploymentCommitInfoReceived struct {
	DeploymentID  int64  `json:"deployment_id"`
	ApplicationID int64  `json:"application_id"`
	CommitHash    string `json:"commit_hash"`
	CommitMessage string `json:"commit_message"`
}
