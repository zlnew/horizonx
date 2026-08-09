package domain

import (
	"context"
	"encoding/json"
	"errors"
)

// ErrAgentOffline is returned when SendCommand targets a server with no
// live agent connection. The HTTP layer maps this to a clear 409 so the
// dashboard can tell "agent is down" from "request failed".
var ErrAgentOffline = errors.New("agent offline")

// AgentCommand is the envelope for a server → agent command (the send path
// of the agent WS). A1 adds the transport + dispatch switch; commands beyond
// "ping" arrive in A2 (logs_tail_start / logs_tail_stop / logs_query).
type AgentCommand struct {
	ID      string          `json:"id"`      // correlation id
	Type    string          `json:"type"`    // ping | logs_tail_start | logs_tail_stop | logs_query
	Payload json.RawMessage `json:"payload"` // per-type payload (may be null)
}

// --- Plane B: container runtime logs (A2) ---

// ContainerLogLine is a single parsed line of `docker compose logs` output.
type ContainerLogLine struct {
	Timestamp string `json:"ts"`      // from --timestamps
	Service   string `json:"service"` // parsed from the compose prefix
	Stream    string `json:"stream"`  // stdout | stderr
	Text      string `json:"text"`
}

// ContainerLogChunk is the agent → server unit. Seq is monotonic per stream;
// the client renders an inline "⚠ N chunks dropped" marker when a gap is
// detected (the hub drops silently under load — this is the honesty layer).
type ContainerLogChunk struct {
	StreamID      string             `json:"stream_id"`
	ApplicationID int64              `json:"application_id"`
	Seq           uint64             `json:"seq"` // monotonic per stream
	Lines         []ContainerLogLine `json:"lines"`
	EOF           bool               `json:"eof"`
	Error         string             `json:"error,omitempty"`
}

// LogsTailStartPayload is the payload of the logs_tail_start command.
type LogsTailStartPayload struct {
	StreamID      string `json:"stream_id"`
	ApplicationID int64  `json:"application_id"`
	DirName       string `json:"dir_name"` // resolves to the app workdir
	Service       string `json:"service"`  // "" = all services
	Tail          int    `json:"tail"`     // initial backlog, default 200
}

// LogsTailStopPayload is the payload of the logs_tail_stop command.
type LogsTailStopPayload struct {
	StreamID string `json:"stream_id"`
}

// LogsQueryPayload is the payload of the logs_query command.
type LogsQueryPayload struct {
	QueryID       string  `json:"query_id"`
	ApplicationID int64   `json:"application_id"`
	DirName       string  `json:"dir_name"`
	Service       string  `json:"service"`
	Since         *string `json:"since"` // RFC3339 or a Docker duration ("1h")
	Until         *string `json:"until"`
	Tail          int     `json:"tail"` // hard cap, default 1000, max 5000
}

// LogsTailRequest is the HTTP body for POST /applications/{id}/logs/tail.
type LogsTailRequest struct {
	Service string `json:"service" validate:"omitempty,max=100"`
	Tail    int    `json:"tail" validate:"omitempty,min=1,max=5000"` // initial backlog
}

// LogsTailStopRequest is the HTTP body for POST /applications/{id}/logs/tail/stop.
type LogsTailStopRequest struct {
	StreamID string `json:"stream_id" validate:"required"`
}

// LogsQueryRequest is the HTTP body for POST /applications/{id}/logs/query.
type LogsQueryRequest struct {
	Service string  `json:"service" validate:"omitempty,max=100"`
	Since   *string `json:"since" validate:"omitempty,max=64"`
	Until   *string `json:"until" validate:"omitempty,max=64"`
	Tail    int     `json:"tail" validate:"omitempty,min=1,max=5000"` // hard cap
}

// ContainerLogService is the server-side entry point for live container
// logs. TailLogs/QueryLogs return immediately with an ID; results arrive
// over the userws channel `app_logs:{appID}`.
type ContainerLogService interface {
	TailLogs(ctx context.Context, appID int64, req LogsTailRequest) (streamID string, err error)
	StopTailLogs(ctx context.Context, appID int64, streamID string) error
	QueryLogs(ctx context.Context, appID int64, req LogsQueryRequest) (queryID string, err error)
}
