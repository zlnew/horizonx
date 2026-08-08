package domain

import (
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
