package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrLogNotFound = errors.New("log not found")

type (
	LogSource string
	LogLevel  string
	LogAction string
	LogStep   string
	LogStream string
)

const (
	LogAgent  LogSource = "agent"
	LogServer LogSource = "server"
)

const (
	LogDebug LogLevel = "debug"
	LogInfo  LogLevel = "info"
	LogWarn  LogLevel = "warn"
	LogError LogLevel = "error"
	LogFatal LogLevel = "fatal"
)

const (
	ActionAppDeploy      LogAction = "app_deploy"
	ActionAppStart       LogAction = "app_start"
	ActionAppStop        LogAction = "app_stop"
	ActionAppRestart     LogAction = "app_restart"
	ActionAppHealthCheck LogAction = "app_health_check"
)

const (
	StepGitClone          LogStep = "git_clone"
	StepDockerBuild       LogStep = "docker_build"
	StepDockerStart       LogStep = "docker_start"
	StepDockerStop        LogStep = "docker_stop"
	StepDockerRestart     LogStep = "docker_restart"
	StepDockerHealthCheck LogStep = "docker_health_check"
)

const (
	StreamStdout LogStream = "stdout"
	StreamStderr LogStream = "stderr"
)

type Log struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Level     LogLevel  `json:"level"`
	Source    LogSource `json:"source"`
	Action    LogAction `json:"action"`
	TraceID   uuid.UUID `json:"trace_id"`

	JobID         *int64     `json:"job_id"`
	ServerID      *uuid.UUID `json:"server_id"`
	ApplicationID *int64     `json:"application_id"`
	DeploymentID  *int64     `json:"deployment_id"`

	Message string      `json:"message"`
	Context *LogContext `json:"context"`

	CreatedAt time.Time `json:"created_at"`
}

type LogListOptions struct {
	ListOptions
	TraceID       *uuid.UUID `json:"trace_id,omitempty"`
	JobID         *int64     `json:"job_id,omitempty"`
	ServerID      *uuid.UUID `json:"server_id,omitempty"`
	ApplicationID *int64     `json:"application_id,omitempty"`
	DeploymentID  *int64     `json:"deployment_id,omitempty"`
	Levels        []string   `json:"levels,omitempty"`
	Sources       []string   `json:"sources,omitempty"`
	Actions       []string   `json:"actions,omitempty"`
}

type LogContext struct {
	Step   LogStep   `json:"step,omitempty"`
	Stream LogStream `json:"stream,omitempty"`
	Line   string    `json:"line,omitempty"`

	ExitCode *int   `json:"exit_code,omitempty"`
	Latency  *int64 `json:"latency_ms,omitempty"`
	Status   string `json:"status,omitempty"`
}

type LogEmitRequest struct {
	Timestamp time.Time `json:"timestamp"`
	Level     LogLevel  `json:"level"`
	Source    LogSource `json:"source"`
	Action    LogAction `json:"action"`
	TraceID   uuid.UUID `json:"trace_id"`

	JobID         *int64     `json:"job_id"`
	ServerID      *uuid.UUID `json:"server_id"`
	ApplicationID *int64     `json:"application_id"`
	DeploymentID  *int64     `json:"deployment_id"`

	Message string      `json:"message"`
	Context *LogContext `json:"context"`
}

type LogRepository interface {
	List(ctx context.Context, opts LogListOptions) ([]*Log, int64, error)
	Create(ctx context.Context, l *Log) (*Log, error)
}

type LogService interface {
	List(ctx context.Context, opts LogListOptions) (*ListResult[*Log], error)
	Create(ctx context.Context, l *Log) (*Log, error)
}
