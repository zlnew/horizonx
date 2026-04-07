package domain

import "github.com/google/uuid"

type AppInfo struct {
	ApplicationID int64  `json:"application_id"`
	AppDir        string `json:"app_dir"`
}

type DeployAppPayload struct {
	ApplicationID int64             `json:"application_id"`
	DeploymentID  int64             `json:"deployment_id"`
	AppDir        string            `json:"app_dir"`
	RepoURL       string            `json:"repo_url"`
	Branch        string            `json:"branch"`
	EnvVars       map[string]string `json:"env_vars,omitempty"`
}

type StartAppPayload = AppInfo

type StopAppPayload = AppInfo

type RestartAppPayload = AppInfo

type AppHealthCheckPayload struct {
	ServerID     uuid.UUID `json:"server_id"`
	Applications []AppInfo `json:"applications"`
}
