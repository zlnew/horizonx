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
	DeploymentDeploying DeploymentStatus = "deploying"
	DeploymentSuccess   DeploymentStatus = "success"
	DeploymentFailed    DeploymentStatus = "failed"
)

type Deployment struct {
	ID            int64            `json:"id"`
	ApplicationID int64            `json:"application_id"`
	Branch        string           `json:"branch"`
	CommitHash    *string          `json:"commit_hash,omitempty"`
	CommitMessage *string          `json:"commit_message,omitempty"`
	Status        DeploymentStatus `json:"status"`
	BuildLogs     *string          `json:"build_logs,omitempty"`
	TriggeredAt   time.Time        `json:"triggered_at"`
	StartedAt     *time.Time       `json:"started_at,omitempty"`
	FinishedAt    *time.Time       `json:"finished_at,omitempty"`
	DeployedBy    *int64           `json:"deployed_by,omitempty"`

	Deployer *User `json:"deployer,omitempty"`
}

type DeploymentListOptions struct {
	ListOptions
	ApplicationID *int64   `json:"application_id,omitempty"`
	DeployedBy    *int64   `json:"deployed_by,omitempty"`
	Statuses      []string `json:"statuses,omitempty"`
}

type DeploymentCreateRequest struct {
	ApplicationID int64  `json:"application_id"`
	Branch        string `json:"branch"`
	DeployedBy    *int64 `json:"deployed_by,omitempty"`
}

type DeploymentCommitInfoRequest = struct {
	CommitHash    string `json:"commit_hash"`
	CommitMessage string `json:"commit_message"`
}

type DeploymentLogsRequest struct {
	Logs      string `json:"logs"`
	IsPartial bool   `json:"is_partial"`
}

type DeploymentRepository interface {
	List(ctx context.Context, opts DeploymentListOptions) ([]*Deployment, int64, error)
	GetByID(ctx context.Context, deploymentID int64) (*Deployment, error)
	Create(ctx context.Context, deployment *Deployment) (*Deployment, error)
	Start(ctx context.Context, deploymentID int64) error
	Finish(ctx context.Context, deploymentID int64) error
	UpdateStatus(ctx context.Context, deploymentID int64, status DeploymentStatus) (*Deployment, error)
	UpdateCommitInfo(ctx context.Context, deploymentID int64, commitHash string, commitMessage string) (*Deployment, error)
	UpdateLogs(ctx context.Context, deploymentID int64, logs string, isPartial bool) (*Deployment, error)
}

type DeploymentService interface {
	List(ctx context.Context, opts DeploymentListOptions) (*ListResult[*Deployment], error)
	GetByID(ctx context.Context, deploymentID int64) (*Deployment, error)
	Create(ctx context.Context, req DeploymentCreateRequest) (*Deployment, error)
	Start(ctx context.Context, deploymentID int64) error
	Finish(ctx context.Context, deploymentID int64) error
	UpdateStatus(ctx context.Context, deploymentID int64, status DeploymentStatus) error
	UpdateCommitInfo(ctx context.Context, deploymentID int64, commitHash string, commitMessage string) error
	UpdateLogs(ctx context.Context, deploymentID int64, logs string, isPartial bool) error
}
