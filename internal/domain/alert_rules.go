package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrAlertRuleNotFound    = errors.New("alert rule not found")
	ErrAlertHistoryNotFound = errors.New("alert history not found")
)

type AlertScope string

const (
	AlertScopeServer AlertScope = "server"
	AlertScopeApp    AlertScope = "app"
	AlertScopeGlobal AlertScope = "global"
)

// AlertCondition is the kind of signal a rule evaluates. It maps 1:1 to the
// `source` column: 'metric' rules compare a metrics field against a
// threshold, 'health' rules react to an application health report, 'offline'
// rules fire when a server goes offline.
type AlertCondition string

const (
	ConditionMetric  AlertCondition = "metric"
	ConditionHealth  AlertCondition = "health"
	ConditionOffline AlertCondition = "offline"
)

type AlertOperator string

const (
	AlertOperatorGT  AlertOperator = ">"
	AlertOperatorGTE AlertOperator = ">="
	AlertOperatorLT  AlertOperator = "<"
	AlertOperatorLTE AlertOperator = "<="
)

type AlertSeverity string

const (
	AlertSeverityInfo     AlertSeverity = "info"
	AlertSeverityWarning  AlertSeverity = "warning"
	AlertSeverityCritical AlertSeverity = "critical"
)

type AlertState string

const (
	AlertStateFiring   AlertState = "firing"
	AlertStateResolved AlertState = "resolved"
)

// Supported metric_path values the evaluator can resolve against
// domain.Metrics. Keep in sync with internal/alerting/evaluator.go.
const (
	MetricPathCPUUsagePercent    = "cpu.usage_percent"
	MetricPathCPUTemperature     = "cpu.temperature"
	MetricPathMemoryUsagePercent = "memory.usage_percent"
	MetricPathMemoryUsedGB       = "memory.used_gb"
	MetricPathDiskUsagePercent   = "disk.usage_percent"
	MetricPathNetworkRXSpeedMBs  = "network.rx_speed_mbs"
	MetricPathNetworkTXSpeedMBs  = "network.tx_speed_mbs"
	MetricPathUptimeSeconds      = "uptime_seconds"
)

var validMetricPaths = map[string]bool{
	MetricPathCPUUsagePercent:    true,
	MetricPathCPUTemperature:     true,
	MetricPathMemoryUsagePercent: true,
	MetricPathMemoryUsedGB:       true,
	MetricPathDiskUsagePercent:   true,
	MetricPathNetworkRXSpeedMBs:  true,
	MetricPathNetworkTXSpeedMBs:  true,
	MetricPathUptimeSeconds:      true,
}

// ValidMetricPath reports whether p is a resolvable metric_path.
func ValidMetricPath(p string) bool {
	return validMetricPaths[p]
}

type AlertRule struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`

	Scope    AlertScope `json:"scope"`
	ServerID *uuid.UUID `json:"server_id,omitempty"`
	AppID    *int64     `json:"app_id,omitempty"`

	Source AlertCondition `json:"source"`

	// Metric rule parameters (source = 'metric').
	MetricPath string        `json:"metric_path,omitempty"`
	Operator   AlertOperator `json:"operator,omitempty"`
	Threshold  *float64      `json:"threshold,omitempty"`

	// Health rule parameter (source = 'health').
	TargetStatus ApplicationStatus `json:"target_status,omitempty"`

	ForDuration int `json:"for_duration"` // debounce: breach must persist N seconds
	Cooldown    int `json:"cooldown"`     // min seconds between two firings

	Severity AlertSeverity `json:"severity"`
	Enabled  bool          `json:"enabled"`

	SilencedUntil *time.Time `json:"silenced_until,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type AlertRuleListOptions struct {
	ListOptions
	Scope    *AlertScope     `json:"scope"`
	ServerID *uuid.UUID      `json:"server_id"`
	AppID    *int64          `json:"app_id"`
	Source   *AlertCondition `json:"source"`
}

type AlertRuleSaveRequest struct {
	Name         string            `json:"name" validate:"required,min=3,max=100"`
	Scope        AlertScope        `json:"scope" validate:"required,oneof=server app global"`
	ServerID     *uuid.UUID        `json:"server_id"`
	AppID        *int64            `json:"app_id"`
	Source       AlertCondition    `json:"source" validate:"required,oneof=metric health offline"`
	MetricPath   string            `json:"metric_path"`
	Operator     AlertOperator     `json:"operator" validate:"omitempty,oneof=> >= < <="`
	Threshold    *float64          `json:"threshold"`
	TargetStatus ApplicationStatus `json:"target_status"`
	ForDuration  int               `json:"for_duration"`
	Cooldown     int               `json:"cooldown"`
	Severity     AlertSeverity     `json:"severity" validate:"required,oneof=info warning critical"`
	Enabled      *bool             `json:"enabled"` // nil on create => enabled
}

// AlertHistory records one fire/resolution for a (rule, server, app) group.
// A row with state='firing' and no later resolve is the "active" alert.
type AlertHistory struct {
	ID     int64 `json:"id"`
	RuleID int64 `json:"rule_id"`
	// RuleName / ServerName / AppName are populated by list/get queries via
	// joins; they are not persisted on the history row itself.
	RuleName   string    `json:"rule_name,omitempty"`
	ServerID   uuid.UUID `json:"server_id"`
	ServerName string    `json:"server_name,omitempty"`
	AppID      *int64    `json:"app_id,omitempty"`
	AppName    string    `json:"app_name,omitempty"`

	Severity AlertSeverity `json:"severity"`
	State    AlertState    `json:"state"`

	Value   *float64 `json:"value,omitempty"`
	Message string   `json:"message"`

	Acked         bool       `json:"acked"`
	SilencedUntil *time.Time `json:"silenced_until,omitempty"`

	FirstFiredAt time.Time  `json:"first_fired_at"`
	ResolvedAt   *time.Time `json:"resolved_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

type AlertHistoryListOptions struct {
	ListOptions
	State    *AlertState    `json:"state"`
	Severity *AlertSeverity `json:"severity"`
	Acked    *bool          `json:"acked"`
	ServerID *uuid.UUID     `json:"server_id"`
	AppID    *int64         `json:"app_id"`
}

type AlertRuleRepository interface {
	List(ctx context.Context, opts AlertRuleListOptions) ([]*AlertRule, int64, error)
	Get(ctx context.Context, ruleID int64) (*AlertRule, error)
	Create(ctx context.Context, rule *AlertRule) (*AlertRule, error)
	Update(ctx context.Context, rule *AlertRule, ruleID int64) error
	Delete(ctx context.Context, ruleID int64) error
	// ListEnabled returns all enabled rules for the evaluator. Rules are
	// re-read live per event so edits and silences apply without restart.
	ListEnabled(ctx context.Context) ([]AlertRule, error)
	SetSilenced(ctx context.Context, ruleID int64, until *time.Time) error
}

type AlertHistoryRepository interface {
	ListActive(ctx context.Context, opts AlertHistoryListOptions) ([]*AlertHistory, int64, error)
	List(ctx context.Context, opts AlertHistoryListOptions) ([]*AlertHistory, int64, error)
	Get(ctx context.Context, historyID int64) (*AlertHistory, error)
	Create(ctx context.Context, alert *AlertHistory) (*AlertHistory, error)
	Resolve(ctx context.Context, historyID int64) error
	Ack(ctx context.Context, historyID int64) error
	SetSilenced(ctx context.Context, historyID int64, until *time.Time) error
}

type AlertService interface {
	ListRules(ctx context.Context, opts AlertRuleListOptions) (*ListResult[*AlertRule], error)
	GetRule(ctx context.Context, ruleID int64) (*AlertRule, error)
	CreateRule(ctx context.Context, req AlertRuleSaveRequest) (*AlertRule, error)
	UpdateRule(ctx context.Context, req AlertRuleSaveRequest, ruleID int64) (*AlertRule, error)
	DeleteRule(ctx context.Context, ruleID int64) error

	ListActive(ctx context.Context, opts AlertHistoryListOptions) (*ListResult[*AlertHistory], error)
	ListHistory(ctx context.Context, opts AlertHistoryListOptions) (*ListResult[*AlertHistory], error)
	GetHistory(ctx context.Context, historyID int64) (*AlertHistory, error)
	Ack(ctx context.Context, historyID int64) error
	// Silence mutes the rule behind an active alert until `until`; the
	// evaluator skips re-firing that rule while silenced.
	Silence(ctx context.Context, historyID int64, until time.Time) error
}
