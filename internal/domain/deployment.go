package domain

import (
	"context"
	"errors"
	"time"
)

var ErrDeploymentNotFound = errors.New("deployment not found")

type DeploymentStatus string

const (
	DeploymentPending   DeploymentStatus = "pending"
	DeploymentBuilding  DeploymentStatus = "building"
	DeploymentDeploying DeploymentStatus = "deploying"
	DeploymentRunning   DeploymentStatus = "running"
	DeploymentFailed    DeploymentStatus = "failed"
)

type Deployment struct {
	ID            int64            `json:"id"`
	ApplicationID int64            `json:"application_id"`
	JobID         *int64           `json:"job_id,omitempty"`
	CommitHash    *string          `json:"commit_hash,omitempty"`
	CommitMessage *string          `json:"commit_message,omitempty"`
	Status        DeploymentStatus `json:"status"`
	BuildLogs     *string          `json:"build_logs,omitempty"`
	StartedAt     time.Time        `json:"started_at"`
	FinishedAt    *time.Time       `json:"finished_at,omitempty"`

	DeployedBy  *string `json:"deployed_by,omitempty"`
	Branch      string  `json:"branch"`
	Environment string  `json:"environment"`
}

type DeploymentCreateRequest struct {
	ApplicationID int64   `json:"application_id"`
	JobID         *int64  `json:"job_id,omitempty"`
	Branch        string  `json:"branch"`
	Environment   string  `json:"environment"`
	DeployedBy    *string `json:"deployed_by,omitempty"`
}

type DeploymentRepository interface {
	List(ctx context.Context, appID int64, limit int) ([]Deployment, error)
	GetByID(ctx context.Context, deploymentID int64) (*Deployment, error)
	GetLatest(ctx context.Context, appID int64) (*Deployment, error)
	Create(ctx context.Context, deployment *Deployment) (*Deployment, error)
	UpdateStatus(ctx context.Context, deploymentID int64, status DeploymentStatus) error
	UpdateCommitInfo(ctx context.Context, deploymentID int64, commitHash, commitMessage string) error
	UpdateLogs(ctx context.Context, deploymentID int64, logs string) error
	Finish(ctx context.Context, deploymentID int64, status DeploymentStatus, logs string) error
}

type DeploymentService interface {
	List(ctx context.Context, appID int64, limit int) ([]Deployment, error)
	GetByID(ctx context.Context, deploymentID int64) (*Deployment, error)
	GetLatest(ctx context.Context, appID int64) (*Deployment, error)
	Create(ctx context.Context, req DeploymentCreateRequest) (*Deployment, error)
}

type EventDeploymentCreated struct {
	Deployment *Deployment `json:"deployment"`
}

type EventDeploymentStatusChanged struct {
	DeploymentID  int64            `json:"deployment_id"`
	ApplicationID int64            `json:"application_id"`
	Status        DeploymentStatus `json:"status"`
}

type EventDeploymentLogsUpdated struct {
	DeploymentID  int64  `json:"deployment_id"`
	ApplicationID int64  `json:"application_id"`
	Logs          string `json:"logs"`
	IsPartial     bool   `json:"is_partial"`
}

type EventDeploymentCompleted struct {
	DeploymentID  int64            `json:"deployment_id"`
	ApplicationID int64            `json:"application_id"`
	Status        DeploymentStatus `json:"status"`
	Logs          string           `json:"logs"`
}
