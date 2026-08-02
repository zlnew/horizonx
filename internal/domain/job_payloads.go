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

// AppRollbackPayload tells the agent to bring the stack back to a previously
// deployed image tag (P0-4). The image must already exist locally (built by an
// earlier successful deploy of the same commit).
type AppRollbackPayload struct {
	ApplicationID int64  `json:"application_id"`
	DeploymentID  int64  `json:"deployment_id"`
	AppKey        string `json:"app_dir"`
	ImageTag      string `json:"image_tag"`
	EnvVars       map[string]string `json:"env_vars,omitempty"`
}

type AppHealthCheckPayload struct {
	ServerID     uuid.UUID `json:"server_id"`
	Applications []AppInfo `json:"applications"`
}
