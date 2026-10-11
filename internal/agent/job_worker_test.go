package agent

import (
	"context"
	"testing"
	"time"

	"horizonx/internal/agent/executor"
	"horizonx/internal/config"
	"horizonx/internal/domain"
	"horizonx/internal/event"

	"github.com/stretchr/testify/require"
)

// P0-2: deploy/rollback timeouts must be long enough for real builds.
func TestDeployTimeoutIsLongEnoughForRealBuilds(t *testing.T) {
	if d := jobTimeout(domain.JobTypeAppDeploy); d < 10*time.Minute {
		t.Fatalf("deploy timeout %v is too short for real builds (needs >= 10m)", d)
	}
	if d := jobTimeout(domain.JobTypeAppRollback); d < 5*time.Minute {
		t.Fatalf("rollback timeout %v too short", d)
	}
}

// P0-2: the old 30s deploy budget must never come back.
func TestDeployTimeoutNotThirtySeconds(t *testing.T) {
	if d := jobTimeout(domain.JobTypeAppDeploy); d == 30*time.Second {
		t.Fatal("deploy timeout must not be 30s")
	}
}

func TestDefaultTimeoutAppliesToUnknownTypes(t *testing.T) {
	if d := jobTimeout("no_such_job"); d != defaultJobTimeout {
		t.Fatalf("expected default %v, got %v", defaultJobTimeout, d)
	}
}

// newMetricsWorker builds a worker wired for the metrics path only. The HTTP
// client points at an empty base URL, so SendMetrics fails harmlessly and is
// logged through the silent test logger.
func newMetricsWorker(t *testing.T) *JobWorker {
	t.Helper()
	cfg := &config.Config{}
	w := NewJobWorker(cfg, noopTestLogger{}, *NewHttpClient(cfg), executor.Executor{})
	w.SetDroppedMessagesFunc(func() int64 { return 7 })
	return w
}

// C3: the drop counter must actually reach the outgoing metrics payload. This
// drives the real bus subscription rather than the stamp helper, because
// deleting the stamp call from the subscription is otherwise a silent
// regression — no other test covers that wiring.
func TestMetricsSubscriptionStampsDroppedMessages(t *testing.T) {
	w := newMetricsWorker(t)

	bus := event.New()
	w.subscribeMetrics(context.Background(), bus)

	m := &domain.Metrics{}
	bus.Publish("metrics", m)

	require.Equal(t, int64(7), m.DroppedMessages,
		"the metrics subscription must stamp the drop count before sending")
}

// A payload that is not a *domain.Metrics must be ignored, not panic.
func TestMetricsSubscriptionIgnoresOtherPayloads(t *testing.T) {
	w := newMetricsWorker(t)

	bus := event.New()
	w.subscribeMetrics(context.Background(), bus)

	require.NotPanics(t, func() { bus.Publish("metrics", "not-a-metrics-pointer") })
}
