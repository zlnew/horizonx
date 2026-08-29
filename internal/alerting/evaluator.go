// Package alerting evaluates alert rules against live HorizonX bus events
// and publishes firing/resolved alert events for delivery (webhook, future
// channels). It is event-driven: no polling, no timers. Rules are re-read
// from the repository per event, so edits and silences apply without restart.
//
// State tracking is per (rule, server, app) and in-memory:
//   - breachStart: when the condition first breached, for for_duration debounce
//   - firing: the alert_history id of the currently-firing alert (if any)
//   - lastFire: when the rule last fired, for cooldown spacing
//
// A server restart resets this cache; a sustained breach simply re-evaluates
// and re-arms on the next event (acceptable for a personal control plane).
package alerting

import (
	"context"
	"fmt"
	"sync"
	"time"

	"horizonx/internal/domain"
	"horizonx/internal/event"
	"horizonx/internal/logger"

	"github.com/google/uuid"
)

// RuleProvider supplies the enabled rules. Implemented by the postgres
// AlertRuleRepository (domain.AlertRuleRepository.ListEnabled).
type RuleProvider interface {
	ListEnabled(ctx context.Context) ([]domain.AlertRule, error)
}

// HistoryWriter persists fire/resolve transitions. Implemented by the
// postgres AlertHistoryRepository.
type HistoryWriter interface {
	Create(ctx context.Context, alert *domain.AlertHistory) (*domain.AlertHistory, error)
	Resolve(ctx context.Context, historyID int64) error
}

// alertKey uniquely identifies the alert group a rule fires for.
type alertKey struct {
	ruleID   int64
	serverID uuid.UUID
	appID    int64
	hasApp   bool
}

func (k alertKey) appIDPtr() *int64 {
	if !k.hasApp {
		return nil
	}
	v := k.appID
	return &v
}

type Evaluator struct {
	bus      *event.Bus
	log      logger.Logger
	provider RuleProvider
	history  HistoryWriter

	// now is injectable for deterministic debounce/cooldown tests.
	now func() time.Time

	mu       sync.Mutex
	breachAt map[alertKey]time.Time
	firing   map[alertKey]int64
	lastFire map[alertKey]time.Time
}

func New(bus *event.Bus, log logger.Logger) *Evaluator {
	return &Evaluator{
		bus:      bus,
		log:      log,
		now:      time.Now,
		breachAt: make(map[alertKey]time.Time),
		firing:   make(map[alertKey]int64),
		lastFire: make(map[alertKey]time.Time),
	}
}

// WithProvider wires the rule source (required before Start).
func (e *Evaluator) WithProvider(p RuleProvider) *Evaluator {
	e.provider = p
	return e
}

// WithHistory wires the alert history persistence (required before Start).
func (e *Evaluator) WithHistory(h HistoryWriter) *Evaluator {
	e.history = h
	return e
}

// withClock overrides the time source (tests only).
func (e *Evaluator) withClock(f func() time.Time) *Evaluator {
	e.now = f
	return e
}

// Start subscribes the evaluator to the server-side bus topics:
//   - server_metrics_received (domain.Metrics, every 10s per online server)
//   - server_status_changed   (domain.EventServerStatusChanged, online/offline)
//   - app_healths             (domain.EventApplicationHealthReported, ~30s)
func (e *Evaluator) Start() {
	e.bus.Subscribe("server_metrics_received", func(event any) {
		if m, ok := event.(domain.Metrics); ok {
			e.HandleMetrics(m)
		}
	})
	e.bus.Subscribe("server_status_changed", func(event any) {
		if evt, ok := event.(domain.EventServerStatusChanged); ok {
			e.HandleStatus(evt)
		}
	})
	e.bus.Subscribe("app_healths", func(event any) {
		if evt, ok := event.(domain.EventApplicationHealthReported); ok {
			e.HandleHealth(evt)
		}
	})
}

// HandleMetrics evaluates metric rules against a metrics broadcast.
func (e *Evaluator) HandleMetrics(m domain.Metrics) {
	for _, rule := range e.loadRules() {
		if rule.Source != domain.ConditionMetric {
			continue
		}
		switch rule.Scope {
		case domain.AlertScopeApp:
			continue // metrics carry no app identity
		case domain.AlertScopeServer:
			if rule.ServerID == nil || *rule.ServerID != m.ServerID {
				continue
			}
		}

		value, ok := resolveMetric(rule.MetricPath, m)
		if !ok {
			continue
		}
		threshold := 0.0
		if rule.Threshold != nil {
			threshold = *rule.Threshold
		}

		key := alertKey{ruleID: rule.ID, serverID: m.ServerID}
		msg := fmt.Sprintf(
			"%s breached: %.2f %s %.2f",
			metricLabel(rule.MetricPath), value, rule.Operator, threshold,
		)
		e.evaluate(rule, key, compare(value, rule.Operator, threshold), &value, msg)
	}
}

// HandleStatus evaluates offline rules on server online/offline transitions.
func (e *Evaluator) HandleStatus(evt domain.EventServerStatusChanged) {
	breached := !evt.IsOnline
	for _, rule := range e.loadRules() {
		if rule.Source != domain.ConditionOffline {
			continue
		}
		switch rule.Scope {
		case domain.AlertScopeApp:
			continue
		case domain.AlertScopeServer:
			if rule.ServerID == nil || *rule.ServerID != evt.ServerID {
				continue
			}
		}

		key := alertKey{ruleID: rule.ID, serverID: evt.ServerID}
		e.evaluate(rule, key, breached, nil, "Server is offline")
	}
}

// HandleHealth evaluates health rules against an application health report.
func (e *Evaluator) HandleHealth(evt domain.EventApplicationHealthReported) {
	rules := e.loadRules()
	for _, report := range evt.Reports {
		for _, rule := range rules {
			if rule.Source != domain.ConditionHealth {
				continue
			}
			switch rule.Scope {
			case domain.AlertScopeApp:
				if rule.AppID == nil || *rule.AppID != report.ApplicationID {
					continue
				}
			case domain.AlertScopeServer:
				if rule.ServerID == nil || *rule.ServerID != evt.ServerID {
					continue
				}
			}

			key := alertKey{ruleID: rule.ID, serverID: evt.ServerID, appID: report.ApplicationID, hasApp: true}
			breached := report.Status == rule.TargetStatus
			msg := fmt.Sprintf("Application %d is %s", report.ApplicationID, report.Status)
			e.evaluate(rule, key, breached, nil, msg)
		}
	}
}

// evaluate drives the state machine for one (rule, key) group. It only
// transitions: absent→firing (fires) and firing→resolved (resolves); an
// ongoing breach does NOT re-fire. Published events are the single signal the
// webhook notifier consumes.
func (e *Evaluator) evaluate(rule domain.AlertRule, key alertKey, breached bool, value *float64, message string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := e.now().UTC()

	if breached {
		// Silenced rules never fire; silence pauses debounce progress too.
		if rule.SilencedUntil != nil && now.Before(*rule.SilencedUntil) {
			delete(e.breachAt, key)
			return
		}

		start, ok := e.breachAt[key]
		if !ok {
			start = now
			e.breachAt[key] = start
		}

		if e.firing[key] != 0 {
			return // already firing — one notification per transition
		}

		if rule.ForDuration > 0 && now.Sub(start) < time.Duration(rule.ForDuration)*time.Second {
			return // breach must persist before firing
		}

		// Cooldown: minimum gap between two firings of the same rule.
		if rule.Cooldown > 0 {
			if last, ok := e.lastFire[key]; ok && now.Sub(last) < time.Duration(rule.Cooldown)*time.Second {
				return
			}
		}

		alert := &domain.AlertHistory{
			RuleID:       rule.ID,
			ServerID:     key.serverID,
			AppID:        key.appIDPtr(),
			Severity:     rule.Severity,
			State:        domain.AlertStateFiring,
			Value:        value,
			Message:      message,
			FirstFiredAt: now,
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		created, err := e.history.Create(ctx, alert)
		cancel()
		if err != nil {
			e.log.Error("alerting: failed to persist firing alert", "rule_id", rule.ID, "error", err)
			return
		}
		e.lastFire[key] = now
		e.firing[key] = created.ID

		e.bus.Publish("alert_fired", domain.EventAlertFired{
			RuleID:   rule.ID,
			RuleName: rule.Name,
			ServerID: key.serverID,
			AppID:    key.appIDPtr(),
			Severity: rule.Severity,
			Message:  message,
			Value:    value,
		})
		return
	}

	// Condition cleared.
	delete(e.breachAt, key)
	if id := e.firing[key]; id != 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := e.history.Resolve(ctx, id)
		cancel()
		if err != nil {
			e.log.Warn("alerting: failed to resolve alert", "rule_id", rule.ID, "history_id", id, "error", err)
			return
		}
		delete(e.firing, key)
		e.bus.Publish("alert_resolved", domain.EventAlertResolved{
			RuleID:   rule.ID,
			RuleName: rule.Name,
			ServerID: key.serverID,
			AppID:    key.appIDPtr(),
		})
	}
}

func (e *Evaluator) loadRules() []domain.AlertRule {
	if e.provider == nil {
		return nil
	}
	rules, err := e.provider.ListEnabled(context.Background())
	if err != nil {
		e.log.Error("alerting: failed to load enabled rules", "error", err)
		return nil
	}
	return rules
}

// metricResolvers maps a metric_path into a domain.Metrics field. Keep in
// sync with the constants in internal/domain/alert_rules.go.
var metricResolvers = map[string]func(domain.Metrics) (float64, bool){
	domain.MetricPathCPUUsagePercent: func(m domain.Metrics) (float64, bool) {
		return m.CPU.Usage.EMA, true
	},
	domain.MetricPathCPUTemperature: func(m domain.Metrics) (float64, bool) {
		return m.CPU.Temperature.EMA, true
	},
	domain.MetricPathMemoryUsagePercent: func(m domain.Metrics) (float64, bool) {
		return m.Memory.UsagePercent, true
	},
	domain.MetricPathMemoryUsedGB: func(m domain.Metrics) (float64, bool) {
		return m.Memory.UsedGB, true
	},
	domain.MetricPathDiskUsagePercent: func(m domain.Metrics) (float64, bool) {
		// Highest filesystem utilization across all disks — the meaningful
		// "disk is filling up" signal. Falls back to device util if no
		// filesystems are reported.
		best := -1.0
		found := false
		for _, disk := range m.Disk {
			for _, fs := range disk.Filesystems {
				found = true
				if fs.Percent > best {
					best = fs.Percent
				}
			}
		}
		if found {
			return best, true
		}
		for _, disk := range m.Disk {
			found = true
			if disk.UtilPct.EMA > best {
				best = disk.UtilPct.EMA
			}
		}
		if found {
			return best, true
		}
		return 0, false
	},
	domain.MetricPathNetworkRXSpeedMBs: func(m domain.Metrics) (float64, bool) {
		return m.Network.RXSpeedMBs.Raw, true
	},
	domain.MetricPathNetworkTXSpeedMBs: func(m domain.Metrics) (float64, bool) {
		return m.Network.TXSpeedMBs.Raw, true
	},
	domain.MetricPathUptimeSeconds: func(m domain.Metrics) (float64, bool) {
		return m.UptimeSeconds, true
	},
}

// metricLabels gives human-readable names for notification messages. Keep in
// sync with the constants in internal/domain/alert_rules.go.
var metricLabels = map[string]string{
	domain.MetricPathCPUUsagePercent:    "CPU usage",
	domain.MetricPathCPUTemperature:     "CPU temperature",
	domain.MetricPathMemoryUsagePercent: "Memory usage",
	domain.MetricPathMemoryUsedGB:       "Memory used",
	domain.MetricPathDiskUsagePercent:   "Disk usage",
	domain.MetricPathNetworkRXSpeedMBs:  "Network receive speed",
	domain.MetricPathNetworkTXSpeedMBs:  "Network transmit speed",
	domain.MetricPathUptimeSeconds:      "Uptime",
}

func resolveMetric(path string, m domain.Metrics) (float64, bool) {
	fn, ok := metricResolvers[path]
	if !ok {
		return 0, false
	}
	return fn(m)
}

func metricLabel(path string) string {
	if l, ok := metricLabels[path]; ok {
		return l
	}
	return path
}

func compare(value float64, op domain.AlertOperator, threshold float64) bool {
	switch op {
	case domain.AlertOperatorGT:
		return value > threshold
	case domain.AlertOperatorGTE:
		return value >= threshold
	case domain.AlertOperatorLT:
		return value < threshold
	case domain.AlertOperatorLTE:
		return value <= threshold
	}
	return false
}
