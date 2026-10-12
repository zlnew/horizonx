package alerting

import (
	"context"
	"sync"
	"testing"
	"time"

	"horizonx/internal/domain"
	"horizonx/internal/event"
	"horizonx/internal/logger"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- fakes ----

type discardLogger struct{}

func (discardLogger) Debug(string, ...any) {}
func (discardLogger) Info(string, ...any)  {}
func (discardLogger) Warn(string, ...any)  {}
func (discardLogger) Error(string, ...any) {}

var _ logger.Logger = discardLogger{}

type fakeRuleProvider struct {
	mu    sync.RWMutex
	rules []domain.AlertRule
}

func (f *fakeRuleProvider) ListEnabled(context.Context) ([]domain.AlertRule, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.rules, nil
}

func (f *fakeRuleProvider) set(rules []domain.AlertRule) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rules = rules
}

type fakeHistory struct {
	mu       sync.Mutex
	nextID   int64
	created  []*domain.AlertHistory
	resolved []int64
	listErr  error
}

func (f *fakeHistory) Create(_ context.Context, alert *domain.AlertHistory) (*domain.AlertHistory, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	alert.ID = f.nextID
	alert.CreatedAt = time.Now().UTC()
	f.created = append(f.created, alert)
	return alert, nil
}

func (f *fakeHistory) Resolve(_ context.Context, historyID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resolved = append(f.resolved, historyID)
	for _, a := range f.created {
		if a.ID == historyID {
			a.State = domain.AlertStateResolved
		}
	}
	return nil
}

func (f *fakeHistory) ListActive(_ context.Context, opts domain.AlertHistoryListOptions) ([]*domain.AlertHistory, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, 0, f.listErr
	}
	var out []*domain.AlertHistory
	for _, a := range f.created {
		if a.State == domain.AlertStateFiring {
			out = append(out, a)
		}
	}
	return out, int64(len(out)), nil
}

func (f *fakeHistory) firing() []*domain.AlertHistory {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*domain.AlertHistory
	for _, a := range f.created {
		if a.State == domain.AlertStateFiring {
			out = append(out, a)
		}
	}
	return out
}

func (f *fakeHistory) countCreated() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.created)
}

type eventRecorder struct {
	mu       sync.Mutex
	fired    []domain.EventAlertFired
	resolved []domain.EventAlertResolved
}

func (r *eventRecorder) register(bus *event.Bus) {
	bus.Subscribe("alert_fired", func(event any) {
		if e, ok := event.(domain.EventAlertFired); ok {
			r.mu.Lock()
			r.fired = append(r.fired, e)
			r.mu.Unlock()
		}
	})
	bus.Subscribe("alert_resolved", func(event any) {
		if e, ok := event.(domain.EventAlertResolved); ok {
			r.mu.Lock()
			r.resolved = append(r.resolved, e)
			r.mu.Unlock()
		}
	})
}

func (r *eventRecorder) firedCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.fired)
}

func (r *eventRecorder) resolvedCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.resolved)
}

// ---- helpers ----

func newTestEvaluator(t *testing.T, clock *time.Time, provider *fakeRuleProvider, history *fakeHistory) (*Evaluator, *event.Bus) {
	t.Helper()

	bus := event.New()

	e := New(bus, discardLogger{}).
		WithProvider(provider).
		WithHistory(history).
		WithHistoryReader(history).
		withClock(func() time.Time { return *clock })

	e.Start()
	return e, bus
}

func metricWithCPU(serverID uuid.UUID, pct float64) domain.Metrics {
	return domain.Metrics{
		ServerID:   serverID,
		CPU:        domain.CPUMetric{Usage: domain.Signal{EMA: pct, Raw: pct}},
		RecordedAt: time.Now().UTC(),
	}
}

func baseMetricRule() domain.AlertRule {
	th := 90.0
	return domain.AlertRule{
		ID:         1,
		Name:       "high cpu",
		Scope:      domain.AlertScopeGlobal,
		Source:     domain.ConditionMetric,
		MetricPath: domain.MetricPathCPUUsagePercent,
		Operator:   domain.AlertOperatorGT,
		Threshold:  &th,
		Severity:   domain.AlertSeverityWarning,
		Enabled:    true,
	}
}

// ---- metrics ----

func TestMetricRuleFiresAndResolves(t *testing.T) {
	now := time.Now().UTC()
	clock := &now
	provider := &fakeRuleProvider{}
	provider.set([]domain.AlertRule{baseMetricRule()})
	history := &fakeHistory{}
	_, bus := newTestEvaluator(t, clock, provider, history)
	rec := &eventRecorder{}
	rec.register(bus)

	m := metricWithCPU(uuid.New(), 95)
	bus.Publish("server_metrics_received", m)

	require.Equal(t, 1, rec.firedCount(), "expected one fired event")
	require.Len(t, history.firing(), 1, "expected one active alert row")
	assert.Equal(t, 95.0, *history.firing()[0].Value)

	// Ongoing breach must not re-fire.
	bus.Publish("server_metrics_received", m)
	assert.Equal(t, 1, rec.firedCount(), "no duplicate firing while breach continues")
	assert.Len(t, history.firing(), 1)

	// Clear → resolve.
	bus.Publish("server_metrics_received", metricWithCPU(m.ServerID, 50))
	require.Equal(t, 1, rec.resolvedCount(), "expected one resolved event")
	assert.Len(t, history.firing(), 0, "no active alerts after resolution")
	assert.Len(t, history.resolved, 1)
}

func TestMetricRuleScopedToServer(t *testing.T) {
	now := time.Now().UTC()
	clock := &now
	rule := baseMetricRule()
	rule.Scope = domain.AlertScopeServer
	sid := uuid.New()
	rule.ServerID = &sid

	provider := &fakeRuleProvider{}
	provider.set([]domain.AlertRule{rule})
	history := &fakeHistory{}
	_, bus := newTestEvaluator(t, clock, provider, history)
	rec := &eventRecorder{}
	rec.register(bus)

	// Different server → no fire.
	bus.Publish("server_metrics_received", metricWithCPU(uuid.New(), 95))
	assert.Equal(t, 0, rec.firedCount(), "server-scoped rule must not fire for other servers")

	// Matching server → fire.
	bus.Publish("server_metrics_received", metricWithCPU(sid, 95))
	require.Equal(t, 1, rec.firedCount())
}

func TestForDurationDebounce(t *testing.T) {
	clock := time.Now().UTC()
	cp := &clock

	rule := baseMetricRule()
	rule.ForDuration = 30
	provider := &fakeRuleProvider{}
	provider.set([]domain.AlertRule{rule})
	history := &fakeHistory{}
	_, bus := newTestEvaluator(t, cp, provider, history)
	rec := &eventRecorder{}
	rec.register(bus)

	sid := uuid.New()

	// Breach starts at t0; debounce window is 30s.
	bus.Publish("server_metrics_received", metricWithCPU(sid, 95))
	assert.Equal(t, 0, rec.firedCount(), "must not fire before for_duration elapses")

	// Clearing at t0+10s → debounce discarded, no history ever created.
	*cp = (*cp).Add(10 * time.Second)
	bus.Publish("server_metrics_received", metricWithCPU(sid, 50))
	assert.Equal(t, 0, rec.firedCount())
	assert.Equal(t, 0, history.countCreated(), "short breach must not persist an alert")

	// Re-breach and hold past the window → fires.
	*cp = (*cp).Add(20 * time.Second)
	bus.Publish("server_metrics_received", metricWithCPU(sid, 95))
	*cp = (*cp).Add(31 * time.Second)
	bus.Publish("server_metrics_received", metricWithCPU(sid, 96))
	require.Equal(t, 1, rec.firedCount(), "breach held past for_duration must fire")
	assert.Len(t, history.firing(), 1)
}

func TestCooldownSpacesFirings(t *testing.T) {
	clock := time.Now().UTC()
	cp := &clock

	rule := baseMetricRule()
	rule.Cooldown = 60
	provider := &fakeRuleProvider{}
	provider.set([]domain.AlertRule{rule})
	history := &fakeHistory{}
	_, bus := newTestEvaluator(t, cp, provider, history)
	rec := &eventRecorder{}
	rec.register(bus)

	sid := uuid.New()

	// Fire 1.
	bus.Publish("server_metrics_received", metricWithCPU(sid, 95))
	require.Equal(t, 1, rec.firedCount())

	// Clear, then re-breach 10s later → cooldown suppresses re-fire.
	bus.Publish("server_metrics_received", metricWithCPU(sid, 50))
	*cp = (*cp).Add(10 * time.Second)
	bus.Publish("server_metrics_received", metricWithCPU(sid, 95))
	assert.Equal(t, 1, rec.firedCount(), "cooldown must suppress re-fire within the window")
	assert.Equal(t, 1, history.countCreated(), "suppressed re-fire must not create a second alert")

	// After cooldown elapses, still breached → fires again.
	*cp = (*cp).Add(55 * time.Second)
	bus.Publish("server_metrics_received", metricWithCPU(sid, 95))
	require.Equal(t, 2, rec.firedCount(), "cooldown elapsed → must fire again")
	assert.Len(t, history.firing(), 1)
}

// ---- offline ----

func TestOfflineRuleFiresOnDisconnect(t *testing.T) {
	now := time.Now().UTC()
	clock := &now

	rule := domain.AlertRule{
		ID:       2,
		Name:     "server offline",
		Scope:    domain.AlertScopeServer,
		Source:   domain.ConditionOffline,
		Severity: domain.AlertSeverityCritical,
		Enabled:  true,
	}
	sid := uuid.New()
	rule.ServerID = &sid

	provider := &fakeRuleProvider{}
	provider.set([]domain.AlertRule{rule})
	history := &fakeHistory{}
	_, bus := newTestEvaluator(t, clock, provider, history)
	rec := &eventRecorder{}
	rec.register(bus)

	bus.Publish("server_status_changed", domain.EventServerStatusChanged{ServerID: sid, IsOnline: false})
	require.Equal(t, 1, rec.firedCount(), "offline event must fire")
	assert.Len(t, history.firing(), 1)

	bus.Publish("server_status_changed", domain.EventServerStatusChanged{ServerID: sid, IsOnline: true})
	require.Equal(t, 1, rec.resolvedCount(), "online event must resolve")
	assert.Len(t, history.firing(), 0)
}

// ---- health ----

func TestHealthRuleFiresOnFailedStatus(t *testing.T) {
	now := time.Now().UTC()
	clock := &now

	appID := int64(9)
	rule := domain.AlertRule{
		ID:           3,
		Name:         "app failed",
		Scope:        domain.AlertScopeApp,
		Source:       domain.ConditionHealth,
		TargetStatus: domain.AppStatusFailed,
		Severity:     domain.AlertSeverityWarning,
		Enabled:      true,
		AppID:        &appID,
	}

	provider := &fakeRuleProvider{}
	provider.set([]domain.AlertRule{rule})
	history := &fakeHistory{}
	_, bus := newTestEvaluator(t, clock, provider, history)
	rec := &eventRecorder{}
	rec.register(bus)

	sid := uuid.New()
	bus.Publish("app_healths", domain.EventApplicationHealthReported{
		ServerID: sid,
		Reports: []domain.ApplicationHealth{
			{ApplicationID: 9, Status: domain.AppStatusFailed},
		},
	})
	require.Equal(t, 1, rec.firedCount(), "failed health report must fire")
	assert.Len(t, history.firing(), 1)

	bus.Publish("app_healths", domain.EventApplicationHealthReported{
		ServerID: sid,
		Reports: []domain.ApplicationHealth{
			{ApplicationID: 9, Status: domain.AppStatusRunning},
		},
	})
	require.Equal(t, 1, rec.resolvedCount(), "recovered health report must resolve")
	assert.Len(t, history.firing(), 0)
}

func TestHealthRuleIgnoresOtherApps(t *testing.T) {
	now := time.Now().UTC()
	clock := &now

	appID := int64(9)
	rule := domain.AlertRule{
		ID:           3,
		Name:         "app failed",
		Scope:        domain.AlertScopeApp,
		Source:       domain.ConditionHealth,
		TargetStatus: domain.AppStatusFailed,
		Severity:     domain.AlertSeverityWarning,
		Enabled:      true,
		AppID:        &appID,
	}

	provider := &fakeRuleProvider{}
	provider.set([]domain.AlertRule{rule})
	history := &fakeHistory{}
	_, bus := newTestEvaluator(t, clock, provider, history)
	rec := &eventRecorder{}
	rec.register(bus)

	bus.Publish("app_healths", domain.EventApplicationHealthReported{
		ServerID: uuid.New(),
		Reports: []domain.ApplicationHealth{
			{ApplicationID: 10, Status: domain.AppStatusFailed},
		},
	})
	assert.Equal(t, 0, rec.firedCount(), "health rule must only fire for its app")
}

// ---- silence ----

func TestSilencedRuleSkipsFire(t *testing.T) {
	now := time.Now().UTC()
	clock := &now

	rule := baseMetricRule()
	future := now.Add(1 * time.Hour)
	rule.SilencedUntil = &future
	provider := &fakeRuleProvider{}
	provider.set([]domain.AlertRule{rule})
	history := &fakeHistory{}
	_, bus := newTestEvaluator(t, clock, provider, history)
	rec := &eventRecorder{}
	rec.register(bus)

	sid := uuid.New()
	bus.Publish("server_metrics_received", metricWithCPU(sid, 95))
	assert.Equal(t, 0, rec.firedCount(), "silenced rule must not fire")
	assert.Equal(t, 0, history.countCreated())

	// Silence expired → fires.
	*clock = (*clock).Add(2 * time.Hour)
	bus.Publish("server_metrics_received", metricWithCPU(sid, 96))
	require.Equal(t, 1, rec.firedCount(), "expired silence must allow firing")
}

// ---- restart rehydration (C1) ----

// restartEvaluator simulates a process restart: a brand-new Evaluator over
// the same persisted history re-runs Start, which must rehydrate.
func restartEvaluator(t *testing.T, clock *time.Time, provider *fakeRuleProvider, history *fakeHistory) (*event.Bus, *eventRecorder) {
	t.Helper()
	bus := event.New()
	New(bus, discardLogger{}).
		WithProvider(provider).
		WithHistory(history).
		WithHistoryReader(history).
		withClock(func() time.Time { return *clock }).
		Start()
	rec := &eventRecorder{}
	rec.register(bus)
	return bus, rec
}

func TestRestartMidBreachDoesNotRefire(t *testing.T) {
	now := time.Now().UTC()
	clock := &now
	provider := &fakeRuleProvider{}
	provider.set([]domain.AlertRule{baseMetricRule()})
	history := &fakeHistory{}
	_, bus := newTestEvaluator(t, clock, provider, history)
	rec := &eventRecorder{}
	rec.register(bus)

	sid := uuid.New()
	bus.Publish("server_metrics_received", metricWithCPU(sid, 95))
	require.Equal(t, 1, rec.firedCount())
	require.Equal(t, 1, history.countCreated())

	// Restart with the breach still active.
	bus2, rec2 := restartEvaluator(t, clock, provider, history)
	bus2.Publish("server_metrics_received", metricWithCPU(sid, 95))
	assert.Equal(t, 1, history.countCreated(), "restart must not insert a second firing row")
	assert.Equal(t, 0, rec2.firedCount(), "restart must not publish a second alert_fired")
}

func TestRestartMidBreachResolvesOriginalRow(t *testing.T) {
	now := time.Now().UTC()
	clock := &now
	provider := &fakeRuleProvider{}
	provider.set([]domain.AlertRule{baseMetricRule()})
	history := &fakeHistory{}
	_, bus := newTestEvaluator(t, clock, provider, history)
	rec := &eventRecorder{}
	rec.register(bus)

	sid := uuid.New()
	bus.Publish("server_metrics_received", metricWithCPU(sid, 95))
	require.Equal(t, 1, rec.firedCount())
	require.Len(t, history.created, 1)
	originalID := history.created[0].ID
	require.NotZero(t, originalID)

	// Restart, then clear the condition.
	bus2, rec2 := restartEvaluator(t, clock, provider, history)
	bus2.Publish("server_metrics_received", metricWithCPU(sid, 50))
	require.Len(t, history.resolved, 1, "restarted evaluator must resolve the original row")
	assert.Equal(t, originalID, history.resolved[0])
	assert.Equal(t, 1, rec2.resolvedCount(), "expected one alert_resolved")
}

func TestRehydrationRestoresCooldown(t *testing.T) {
	now := time.Now().UTC()
	clock := &now
	rule := baseMetricRule()
	rule.Cooldown = 300
	provider := &fakeRuleProvider{}
	provider.set([]domain.AlertRule{rule})
	history := &fakeHistory{}
	_, bus := newTestEvaluator(t, clock, provider, history)
	rec := &eventRecorder{}
	rec.register(bus)

	sid := uuid.New()
	bus.Publish("server_metrics_received", metricWithCPU(sid, 95))
	require.Equal(t, 1, rec.firedCount())
	require.Equal(t, 1, history.countCreated())

	// Restart mid-breach, then clear: the original row resolves.
	bus2, rec2 := restartEvaluator(t, clock, provider, history)
	bus2.Publish("server_metrics_received", metricWithCPU(sid, 50))
	require.Equal(t, 1, rec2.resolvedCount())

	// Immediate re-breach inside the 300s cooldown window must not fire.
	*clock = (*clock).Add(10 * time.Second)
	bus2.Publish("server_metrics_received", metricWithCPU(sid, 95))
	assert.Equal(t, 1, history.countCreated(), "cooldown must survive restart: no second row")
	assert.Equal(t, 0, rec2.firedCount(), "cooldown must survive restart: no second alert_fired")
}

func TestRehydrateReadFailureIsNonFatal(t *testing.T) {
	now := time.Now().UTC()
	clock := &now
	provider := &fakeRuleProvider{}
	provider.set([]domain.AlertRule{baseMetricRule()})
	history := &fakeHistory{listErr: assert.AnError}

	bus := event.New()
	e := New(bus, discardLogger{}).
		WithProvider(provider).
		WithHistory(history).
		WithHistoryReader(history).
		withClock(func() time.Time { return *clock })
	require.NotPanics(t, func() { e.Start() }, "rehydrate failure must not break boot")
	rec := &eventRecorder{}
	rec.register(bus)

	// Evaluator still processes a first-time breach after the failed rehydrate.
	history.listErr = nil
	sid := uuid.New()
	bus.Publish("server_metrics_received", metricWithCPU(sid, 95))
	assert.Equal(t, 1, rec.firedCount(), "evaluator must still fire after failed rehydrate")
	assert.Equal(t, 1, history.countCreated())
}
