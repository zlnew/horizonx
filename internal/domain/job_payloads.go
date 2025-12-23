package domain

type JobCommandPayload struct {
	JobID int64
}

type DeployAppPayload struct {
	ApplicationID int64             `json:"application_id"`
	RepoURL       *string           `json:"repo_url"`
	Branch        string            `json:"branch"`
	EnvVars       map[string]string `json:"env_vars,omitempty"`
}

type StartAppPayload struct {
	ApplicationID int64 `json:"application_id"`
}

type StopAppPayload struct {
	ApplicationID int64 `json:"application_id"`
}

type RestartAppPayload struct {
	ApplicationID int64 `json:"application_id"`
}

const (
	JobTypeDeployApp  = "deploy_app"
	JobTypeStartApp   = "start_app"
	JobTypeStopApp    = "stop_app"
	JobTypeRestartApp = "restart_app"
)
