package domain

import "github.com/google/uuid"

type AppInfo struct {
	ApplicationID int64  `json:"application_id"`
	AppKey        string `json:"app_key"`
}

type AppDeployPayload struct {
	ApplicationID int64             `json:"application_id"`
	DeploymentID  int64             `json:"deployment_id"`
	AppKey        string            `json:"app_dir"`
	RepoURL       string            `json:"repo_url"`
	Branch        string            `json:"branch"`
	EnvVars       map[string]string `json:"env_vars,omitempty"`
}

type AppStartPayload = AppInfo

type AppStopPayload = AppInfo

type AppRestartPayload = AppInfo

type AppDestroyPayload = AppInfo

type AppHealthCheckPayload struct {
	ServerID     uuid.UUID `json:"server_id"`
	Applications []AppInfo `json:"applications"`
}
