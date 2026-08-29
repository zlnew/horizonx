package domain

import "github.com/google/uuid"

// EventAlertFired is published on the bus when a rule transitions into
// firing. The webhook notifier converts it into a notification payload.
type EventAlertFired struct {
	RuleID   int64         `json:"rule_id"`
	RuleName string        `json:"rule_name"`
	ServerID uuid.UUID     `json:"server_id"`
	AppID    *int64        `json:"app_id,omitempty"`
	Severity AlertSeverity `json:"severity"`
	Message  string        `json:"message"`
	Value    *float64      `json:"value,omitempty"`
}

// EventAlertResolved is published on the bus when a firing alert clears.
type EventAlertResolved struct {
	RuleID   int64     `json:"rule_id"`
	RuleName string    `json:"rule_name"`
	ServerID uuid.UUID `json:"server_id"`
	AppID    *int64    `json:"app_id,omitempty"`
}

// EventApplicationHealthReported is published server-side when an agent
// reports application health. The agent itself publishes a raw
// []ApplicationHealth on ITS OWN bus; the server wraps the payload with the
// reporting server id because ApplicationHealth carries no server identity.
// The evaluator subscribes to the "app_healths" topic with this payload.
type EventApplicationHealthReported struct {
	ServerID uuid.UUID           `json:"server_id"`
	Reports  []ApplicationHealth `json:"reports"`
}
