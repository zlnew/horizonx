package domain

import (
	"context"
	"encoding/json"
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
	TriggeredAt   time.Time        `json:"triggered_at"`
	StartedAt     *time.Time       `json:"started_at,omitempty"`
	FinishedAt    *time.Time       `json:"finished_at,omitempty"`
	DeployedBy    *int64           `json:"deployed_by,omitempty"`

	// P3 diff support: env snapshot used for this deploy + link to previous.
	EnvSnapshot          json.RawMessage `json:"env_snapshot,omitempty"`
	PreviousDeploymentID *int64          `json:"previous_deployment_id,omitempty"`
	PreviousCommitHash   *string         `json:"previous_commit_hash,omitempty"`

	Deployer *User `json:"deployer,omitempty"`
	Logs     []Log `json:"logs,omitempty"`
}

// DeploymentDiff is the P3 diff between this deployment and its previous one:
// commit range + env var additions/removals/updates.
type DeploymentDiff struct {
	DeploymentID   int64            `json:"deployment_id"`
	CommitFrom     *string          `json:"commit_from,omitempty"`
	CommitTo       *string          `json:"commit_to,omitempty"`
	CommitMessage  *string          `json:"commit_message,omitempty"`
	HasPrevious    bool             `json:"has_previous"`
	EnvAdditions   []EnvDiffEntry   `json:"env_additions"`
	EnvRemovals    []EnvDiffEntry   `json:"env_removals"`
	EnvUpdates     []EnvDiffEntry   `json:"env_updates"`
}

type EnvDiffEntry struct {
	Key   string `json:"key"`
	Old   string `json:"old,omitempty"`
	New   string `json:"new,omitempty"`
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
	Start(ctx context.Context, deploymentID int64) (*Deployment, error)
	Finish(ctx context.Context, deploymentID int64) (*Deployment, error)
	UpdateStatus(ctx context.Context, deploymentID int64, status DeploymentStatus) (*Deployment, error)
	UpdateCommitInfo(ctx context.Context, deploymentID int64, commitHash string, commitMessage string) (*Deployment, error)
	// P3: snapshot the env vars used for this deployment and link the previous
	// successful deployment (for the diff view).
	UpdateEnvSnapshot(ctx context.Context, deploymentID int64, snapshot map[string]string) (*Deployment, error)
}

type DeploymentService interface {
	List(ctx context.Context, opts DeploymentListOptions) (*ListResult[*Deployment], error)
	GetByID(ctx context.Context, deploymentID int64) (*Deployment, error)
	Create(ctx context.Context, req DeploymentCreateRequest) (*Deployment, error)
	Start(ctx context.Context, deploymentID int64) error
	Finish(ctx context.Context, deploymentID int64) error
	UpdateStatus(ctx context.Context, deploymentID int64, status DeploymentStatus) error
	UpdateCommitInfo(ctx context.Context, deploymentID int64, commitHash string, commitMessage string) error
	// P3: record env snapshot + previous-deployment link, return the diff.
	UpdateEnvSnapshot(ctx context.Context, deploymentID int64, snapshot map[string]string) error
	Diff(ctx context.Context, deploymentID int64) (*DeploymentDiff, error)
}
