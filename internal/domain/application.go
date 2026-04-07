package domain

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	ErrApplicationNotFound  = errors.New("application not found")
	ErrInvalidDockerCompose = errors.New("invalid docker compose configuration")
)

type ApplicationStatus string

const (
	AppStatusDeploying  ApplicationStatus = "deploying"
	AppStatusStarting   ApplicationStatus = "starting"
	AppStatusStopping   ApplicationStatus = "stopping"
	AppStatusRestarting ApplicationStatus = "restarting"
	AppStatusRunning    ApplicationStatus = "running"
	AppStatusStopped    ApplicationStatus = "stopped"
	AppStatusFailed     ApplicationStatus = "failed"
	AppStatusUnknown    ApplicationStatus = "unknown"
)

type Application struct {
	ID               int64             `json:"id"`
	ServerID         uuid.UUID         `json:"server_id"`
	Name             string            `json:"name"`
	RepoName         string            `json:"repo_name"`
	RepoURL          string            `json:"repo_url"`
	SiteURL          string            `json:"site_url,omitempty"`
	Branch           string            `json:"branch"`
	Status           ApplicationStatus `json:"status"`
	LastDeploymentAt *time.Time        `json:"last_deployment_at,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`

	EnvVars *[]EnvironmentVariable `json:"env_vars,omitempty"`
}

type ApplicationListOptions struct {
	ListOptions
	ServerID *uuid.UUID `json:"server_id"`
}

type ApplicationCreateRequest struct {
	ServerID uuid.UUID `json:"server_id" validate:"required"`
	Name     string    `json:"name" validate:"required,min=3,max=100"`
	RepoName string    `json:"repo_name" validate:"required,max=100"`
	RepoURL  string    `json:"repo_url" validate:"required,max=255"`
	SiteURL  string    `json:"site_url" validate:"omitempty,max=255"`
	Branch   string    `json:"branch" validate:"required"`

	EnvVars []EnvironmentVariableRequest `json:"env_vars" validate:"omitempty,dive"`
}

type ApplicationUpdateRequest struct {
	Name    string `json:"name" validate:"required,min=3,max=100"`
	SiteURL string `json:"site_url" validate:"omitempty,max=255"`
	Branch  string `json:"branch" validate:"required"`

	EnvVars []EnvironmentVariableRequest `json:"env_vars" validate:"omitempty,dive"`
}

type ApplicationHealth struct {
	ApplicationID int64             `json:"application_id"`
	Status        ApplicationStatus `json:"status"`
}

type EnvironmentVariable struct {
	ID            int64     `json:"id"`
	ApplicationID int64     `json:"application_id"`
	Key           string    `json:"key"`
	Value         string    `json:"value"`
	IsPreview     bool      `json:"is_preview"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type EnvironmentVariableRequest struct {
	Key       string `json:"key" validate:"required"`
	Value     string `json:"value" validate:"required"`
	IsPreview bool   `json:"is_preview"`
}

type ApplicationRepository interface {
	List(ctx context.Context, opts ApplicationListOptions) ([]*Application, int64, error)
	GetByID(ctx context.Context, appID int64) (*Application, error)
	Create(ctx context.Context, app *Application) (*Application, error)
	Update(ctx context.Context, app *Application, appID int64) error
	UpdateStatus(ctx context.Context, appID int64, status ApplicationStatus) error
	UpdateLastDeployment(ctx context.Context, appID int64) error
	UpdateHealth(ctx context.Context, serverID uuid.UUID, reports []ApplicationHealth) error
	Delete(ctx context.Context, appID int64) error

	SyncEnvVars(ctx context.Context, appID int64, envVars []EnvironmentVariable) error
	ListEnvVars(ctx context.Context, appID int64) ([]EnvironmentVariable, error)
	CreateEnvVar(ctx context.Context, env *EnvironmentVariable) error
	UpdateEnvVar(ctx context.Context, env *EnvironmentVariable) error
	DeleteEnvVar(ctx context.Context, appID int64, key string) error
}

type ApplicationService interface {
	List(ctx context.Context, opts ApplicationListOptions) (*ListResult[*Application], error)
	GetByID(ctx context.Context, appID int64) (*Application, error)
	Create(ctx context.Context, req ApplicationCreateRequest) (*Application, error)
	Update(ctx context.Context, req ApplicationUpdateRequest, appID int64) error
	UpdateStatus(ctx context.Context, appID int64, status ApplicationStatus) error
	UpdateLastDeployment(ctx context.Context, appID int64) error
	UpdateHealth(ctx context.Context, serverID uuid.UUID, reports []ApplicationHealth) error
	Delete(ctx context.Context, appID int64) error

	Deploy(ctx context.Context, appID int64, deployedBy int64) (*Deployment, error)
	Start(ctx context.Context, appID int64) error
	Stop(ctx context.Context, appID int64) error
	Restart(ctx context.Context, appID int64) error

	ListEnvVars(ctx context.Context, appID int64) ([]EnvironmentVariable, error)
	AddEnvVar(ctx context.Context, appID int64, req EnvironmentVariableRequest) error
	UpdateEnvVar(ctx context.Context, appID int64, key string, req EnvironmentVariableRequest) error
	DeleteEnvVar(ctx context.Context, appID int64, key string) error
}

func GetAppDir(app *Application) string {
	return fmt.Sprintf("%s-%d", app.RepoName, app.ID)
}
