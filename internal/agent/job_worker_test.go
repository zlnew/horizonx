package agent

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"horizonx/internal/agent/executor"
	"horizonx/internal/config"
	"horizonx/internal/domain"
	"horizonx/internal/event"
	"horizonx/internal/logger"

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
	w.subscribeMetrics(bus, make(chan *domain.Metrics, 1))

	m := &domain.Metrics{}
	bus.Publish("metrics", m)

	require.Equal(t, int64(7), m.DroppedMessages,
		"the metrics subscription must stamp the drop count before sending")
}

// A payload that is not a *domain.Metrics must be ignored, not panic.
func TestMetricsSubscriptionIgnoresOtherPayloads(t *testing.T) {
	w := newMetricsWorker(t)

	bus := event.New()
	w.subscribeMetrics(bus, make(chan *domain.Metrics, 1))

	require.NotPanics(t, func() { bus.Publish("metrics", "not-a-metrics-pointer") })
}

// hangingTransport blocks every RoundTrip until release is closed, standing in
// for a black-holed server that would otherwise stall the emit path for the
// client's 5-minute timeout. calls counts how many requests actually reached
// the transport.
type hangingTransport struct {
	release chan struct{}
	calls   atomic.Int64
}

func (t *hangingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	t.calls.Add(1)
	<-t.release
	return nil, errors.New("transport released")
}

// newHangingWorker builds a worker whose HTTP client hangs on every request.
func newHangingWorker(t *testing.T, log logger.Logger) (*JobWorker, *hangingTransport) {
	t.Helper()
	tr := &hangingTransport{release: make(chan struct{})}
	t.Cleanup(func() { close(tr.release) })

	cfg := &config.Config{}
	w := NewJobWorker(cfg, log, HttpClient{cfg: cfg, http: &http.Client{Transport: tr}}, executor.Executor{})
	w.SetDroppedMessagesFunc(func() int64 { return 0 })
	return w, tr
}

// The emit path must never perform the metrics HTTP call synchronously: a
// stalled server would otherwise block job execution for the client timeout.
func TestMetricsPublishDoesNotBlockOnHangingHTTP(t *testing.T) {
	w, tr := newHangingWorker(t, noopTestLogger{})

	bus := event.New()
	w.subscribeMetrics(bus, make(chan *domain.Metrics, 500))

	done := make(chan struct{})
	go func() {
		bus.Publish("metrics", &domain.Metrics{})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Publish blocked on the emit path; metrics must be drained off the executor's goroutine")
	}

	require.Equal(t, int64(0), tr.calls.Load(),
		"the emit path must not reach the network synchronously")
}

// When the buffer is full, an extra publish must drop (and report) rather than
// block waiting for the hanging HTTP call.
func TestMetricsPublishDropsUnderBackpressure(t *testing.T) {
	log := &countWarnLogger{}
	w, _ := newHangingWorker(t, log)

	bus := event.New()
	const capacity = 2
	w.subscribeMetrics(bus, make(chan *domain.Metrics, capacity))

	done := make(chan struct{})
	go func() {
		for range capacity + 1 {
			bus.Publish("metrics", &domain.Metrics{})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("publishing past the channel capacity must drop, not block")
	}

	require.Equal(t, 1, log.warns, "the dropped metrics event must be reported in the log")
}

// With a healthy transport the drained request must actually reach the server.
func TestMetricsDeliveredWhenTransportHealthy(t *testing.T) {
	var reqs atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs.Add(1)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	cfg := &config.Config{AgentTargetAPIURL: srv.URL}
	exec := executor.NewExecutorWithDeps(nil, nil, t.TempDir(), noopTestLogger{}, func() *domain.Metrics {
		return &domain.Metrics{}
	})
	w := NewJobWorker(cfg, noopTestLogger{}, *NewHttpClient(cfg), *exec)

	// execute returns only after its drain goroutines finish, so the request
	// has been delivered by the time it returns.
	require.NoError(t, w.execute(context.Background(), domain.Job{Type: domain.JobTypeMetricsCollect}))

	require.Eventually(t, func() bool { return reqs.Load() >= 1 }, time.Second, 10*time.Millisecond,
		"a metrics event must reach the server through the drain goroutine")
}
