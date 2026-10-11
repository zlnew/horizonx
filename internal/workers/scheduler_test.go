package workers

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeLocker records TryLock calls and release invocations. It is safe for the
// goroutine-driven scheduler tests: TryLock is called from a worker goroutine
// while the test reads the counters.
type fakeLocker struct {
	mu       sync.Mutex
	acquired bool
	err      error

	tryLockNames []string
	releases     int
}

func (f *fakeLocker) TryLock(_ context.Context, name string) (func(), bool, error) {
	f.mu.Lock()
	f.tryLockNames = append(f.tryLockNames, name)
	f.mu.Unlock()

	release := func() {
		f.mu.Lock()
		f.releases++
		f.mu.Unlock()
	}

	if f.err != nil {
		return release, false, f.err
	}
	if !f.acquired {
		return release, false, nil
	}
	return release, true, nil
}

func (f *fakeLocker) nameCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.tryLockNames...)
}

func (f *fakeLocker) releaseCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.releases
}

type fakeWorker struct {
	name string
	runs int
	err  error
}

func (f *fakeWorker) Name() string { return f.name }

func (f *fakeWorker) Run(context.Context) error {
	f.runs++
	return f.err
}

// recordingWorker signals on a buffered channel the first time it runs, so a
// goroutine-driven test can wait on it instead of sleeping.
type recordingWorker struct {
	name string
	ran  chan struct{}
}

func (w *recordingWorker) Name() string { return w.name }

func (w *recordingWorker) Run(context.Context) error {
	select {
	case w.ran <- struct{}{}:
	default:
	}
	return nil
}

// fakeReporter records WorkerRunFailed callbacks per worker name.
type fakeReporter struct {
	mu     sync.Mutex
	counts map[string]int
}

func (f *fakeReporter) WorkerRunFailed(worker string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.counts == nil {
		f.counts = map[string]int{}
	}
	f.counts[worker]++
}

func (f *fakeReporter) count(worker string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.counts[worker]
}

// recordingLog captures log messages so tests can assert what was emitted.
type recordingLog struct {
	mu   sync.Mutex
	msgs []string
}

func (l *recordingLog) append(msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.msgs = append(l.msgs, msg)
}

func (l *recordingLog) Debug(msg string, _ ...any) { l.append(msg) }
func (l *recordingLog) Info(msg string, _ ...any)  { l.append(msg) }
func (l *recordingLog) Warn(msg string, _ ...any)  { l.append(msg) }
func (l *recordingLog) Error(msg string, _ ...any) { l.append(msg) }

func (l *recordingLog) count(msg string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, m := range l.msgs {
		if m == msg {
			n++
		}
	}
	return n
}

// flakyWorker fails its first failCount runs, then succeeds.
type flakyWorker struct {
	name      string
	failCount int
	runs      int
}

func (w *flakyWorker) Name() string { return w.name }

func (w *flakyWorker) Run(context.Context) error {
	w.runs++
	if w.runs <= w.failCount {
		return errors.New("transient")
	}
	return nil
}

// newTestScheduler builds a scheduler with a millisecond-scale retry policy so
// retry tests never meaningfully sleep.
func newTestScheduler(locker RunLocker, reporter WorkerFailureReporter) *Scheduler {
	s := NewScheduler(nil, noopLog{}, locker, reporter)
	s.retry = retryPolicy{attempts: 3, base: time.Millisecond, max: 2 * time.Millisecond}
	return s
}

// 1. Not acquired: the worker must never run and release must never be called.
func TestSchedulerDoesNotRunWorkerWhenLockNotAcquired(t *testing.T) {
	locker := &fakeLocker{acquired: false}
	worker := &fakeWorker{name: "metrics_collect"}
	reporter := &fakeReporter{}
	s := newTestScheduler(locker, reporter)

	s.runWorker(context.Background(), worker)

	if worker.runs != 0 {
		t.Errorf("worker ran %d times, want 0 when the lock was not acquired", worker.runs)
	}
	if got := locker.releaseCount(); got != 0 {
		t.Errorf("release called %d times, want 0 when the lock was not acquired", got)
	}
	if got := reporter.count("metrics_collect"); got != 0 {
		t.Errorf("failure reported %d times, want 0 when the lock was not acquired", got)
	}
}

// 2. Acquired: the worker runs and the deferred release fires afterwards.
func TestSchedulerRunsWorkerAndReleasesWhenLockAcquired(t *testing.T) {
	locker := &fakeLocker{acquired: true}
	worker := &fakeWorker{name: "job_reaper"}
	s := newTestScheduler(locker, nil)

	s.runWorker(context.Background(), worker)

	if worker.runs != 1 {
		t.Errorf("worker ran %d times, want 1 when the lock was acquired", worker.runs)
	}
	if got := locker.releaseCount(); got != 1 {
		t.Errorf("release called %d times, want 1 after the run (proves the defer)", got)
	}
}

// 3. Lock error: fail closed. The worker must never run and release must not fire.
func TestSchedulerDoesNotRunWorkerWhenLockErrors(t *testing.T) {
	locker := &fakeLocker{err: errors.New("lock backend down")}
	worker := &fakeWorker{name: "metrics_collect"}
	s := newTestScheduler(locker, nil)

	s.runWorker(context.Background(), worker)

	if worker.runs != 0 {
		t.Errorf("worker ran %d times, want 0 when TryLock errored", worker.runs)
	}
	if got := locker.releaseCount(); got != 0 {
		t.Errorf("release called %d times, want 0 when TryLock errored", got)
	}
}

// 4. Worker error: release must still fire (the defer covers every path).
func TestSchedulerReleasesWhenWorkerReturnsError(t *testing.T) {
	locker := &fakeLocker{acquired: true}
	worker := &fakeWorker{name: "metrics_collect", err: errors.New("worker exploded")}
	s := newTestScheduler(locker, nil)
	// Retry is covered by its own tests; this test pins the release path.
	s.retry.attempts = 1

	s.runWorker(context.Background(), worker)

	if worker.runs != 1 {
		t.Errorf("worker ran %d times, want 1", worker.runs)
	}
	if got := locker.releaseCount(); got != 1 {
		t.Errorf("release called %d times, want 1 after a failing run", got)
	}
}

// 5. Nil locker means no locking and the worker still runs.
func TestSchedulerRunsWorkerWithNilLocker(t *testing.T) {
	worker := &fakeWorker{name: "metrics_collect"}
	s := newTestScheduler(nil, nil)

	s.runWorker(context.Background(), worker)

	if worker.runs != 1 {
		t.Errorf("worker ran %d times, want 1 with a nil locker", worker.runs)
	}
}

// 6. The locker is asked about the worker's own name.
func TestSchedulerPassesWorkerNameToLocker(t *testing.T) {
	locker := &fakeLocker{acquired: true}
	worker := &fakeWorker{name: "application_health_check"}
	s := newTestScheduler(locker, nil)

	s.runWorker(context.Background(), worker)

	calls := locker.nameCalls()
	if len(calls) != 1 {
		t.Fatalf("TryLock called %d times, want 1", len(calls))
	}
	if calls[0] != worker.Name() {
		t.Errorf("TryLock called with %q, want the worker name %q", calls[0], worker.Name())
	}
}

// Guards the wiring: RunByDuration must route through the shared runWorker path
// (and therefore consult the locker), not run the worker inline.
func TestRunByDurationConsultsLocker(t *testing.T) {
	locker := &fakeLocker{acquired: true}
	ran := make(chan struct{}, 1)
	worker := &recordingWorker{name: "tick", ran: ran}
	s := newTestScheduler(locker, nil)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	s.RunByDuration(ctx, 10*time.Millisecond, worker)

	select {
	case <-ran:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not run within 2s of RunByDuration")
	}

	if len(locker.nameCalls()) == 0 {
		t.Error("RunByDuration ran the worker without consulting the locker")
	}
}

// 7. A worker failing twice then succeeding is retried, and a recovered run is
// not an operator-facing failure.
func TestSchedulerRetriesTransientFailureWithoutReporting(t *testing.T) {
	locker := &fakeLocker{acquired: true}
	worker := &flakyWorker{name: "metrics_collect", failCount: 2}
	reporter := &fakeReporter{}
	log := &recordingLog{}
	s := newTestScheduler(locker, reporter)
	s.log = log

	s.runWorker(context.Background(), worker)

	if worker.runs != 3 {
		t.Errorf("worker ran %d times, want 3 (two failures then success)", worker.runs)
	}
	if got := reporter.count("metrics_collect"); got != 0 {
		t.Errorf("failure reported %d times, want 0 when a retry recovered", got)
	}
	if got := log.count("worker failed"); got != 0 {
		t.Errorf(`"worker failed" logged %d times, want 0 when a retry recovered`, got)
	}
}

// 8. A permanently failing worker is retried up to the policy and reported
// exactly once, only after retries are exhausted.
func TestSchedulerReportsFailureOnlyAfterRetriesExhausted(t *testing.T) {
	locker := &fakeLocker{acquired: true}
	worker := &fakeWorker{name: "metrics_collect", err: errors.New("permanent")}
	reporter := &fakeReporter{}
	log := &recordingLog{}
	s := newTestScheduler(locker, reporter)
	s.log = log

	s.runWorker(context.Background(), worker)

	if worker.runs != 3 {
		t.Errorf("worker ran %d times, want 3 (attempts)", worker.runs)
	}
	if got := reporter.count("metrics_collect"); got != 1 {
		t.Errorf("failure reported %d times, want exactly 1 after exhausted retries", got)
	}
	if got := log.count("worker failed"); got != 1 {
		t.Errorf(`"worker failed" logged %d times, want exactly 1`, got)
	}
}

// 9. attempts=1 disables retry: one run, one report.
func TestSchedulerRetryDisabledRunsOnce(t *testing.T) {
	locker := &fakeLocker{acquired: true}
	worker := &fakeWorker{name: "metrics_collect", err: errors.New("permanent")}
	reporter := &fakeReporter{}
	s := newTestScheduler(locker, reporter)
	s.retry.attempts = 1

	s.runWorker(context.Background(), worker)

	if worker.runs != 1 {
		t.Errorf("worker ran %d times, want 1 with retry disabled", worker.runs)
	}
	if got := reporter.count("metrics_collect"); got != 1 {
		t.Errorf("failure reported %d times, want 1 with retry disabled", got)
	}
}

// 10. A context cancelled during the backoff wait stops the retry promptly
// rather than sleeping out the full delay.
func TestSchedulerStopsRetryingWhenContextCancelledDuringBackoff(t *testing.T) {
	locker := &fakeLocker{acquired: true}
	worker := &fakeWorker{name: "metrics_collect", err: errors.New("permanent")}
	s := newTestScheduler(locker, nil)
	s.retry = retryPolicy{attempts: 3, base: 5 * time.Second, max: 5 * time.Second}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	time.AfterFunc(50*time.Millisecond, cancel)

	start := time.Now()
	s.runWorker(ctx, worker)
	elapsed := time.Since(start)

	if elapsed >= 500*time.Millisecond {
		t.Errorf("runWorker took %v, want it to return promptly when the context is cancelled during backoff", elapsed)
	}
	if worker.runs >= 3 {
		t.Errorf("worker ran %d times, want fewer than attempts after cancellation", worker.runs)
	}
}

// 11. The cross-replica lock is taken once per run, not once per attempt, so no
// other replica can slip in between retries.
func TestSchedulerAcquiresLockOnceAcrossRetries(t *testing.T) {
	locker := &fakeLocker{acquired: true}
	worker := &fakeWorker{name: "metrics_collect", err: errors.New("permanent")}
	s := newTestScheduler(locker, nil)

	s.runWorker(context.Background(), worker)

	if got := len(locker.nameCalls()); got != 1 {
		t.Errorf("TryLock called %d times, want exactly 1 across all retries", got)
	}
	if got := locker.releaseCount(); got != 1 {
		t.Errorf("release called %d times, want exactly 1 across all retries", got)
	}
}

// 12. A nil reporter must be safe.
func TestSchedulerNilReporterDoesNotPanic(t *testing.T) {
	locker := &fakeLocker{acquired: true}
	worker := &fakeWorker{name: "metrics_collect", err: errors.New("permanent")}
	s := newTestScheduler(locker, nil)
	s.retry.attempts = 1

	s.runWorker(context.Background(), worker) // must not panic
}

// 13. retryDelay is exponential from base, non-decreasing, capped at max, and
// overflow-safe for a large n.
func TestRetryDelay(t *testing.T) {
	const base = time.Second
	const max = 8 * time.Second

	if got := retryDelay(0, base, max); got != base {
		t.Errorf("retryDelay(0) = %v, want base %v", got, base)
	}
	if got := retryDelay(-1, base, max); got != base {
		t.Errorf("retryDelay(-1) = %v, want base %v (n<0 treated as 0)", got, base)
	}

	prev := time.Duration(0)
	for n := 0; n < 6; n++ {
		got := retryDelay(n, base, max)
		if got < prev {
			t.Errorf("retryDelay(%d) = %v, less than previous %v (sequence must be non-decreasing)", n, got, prev)
		}
		if got > max {
			t.Errorf("retryDelay(%d) = %v, exceeds max %v", n, got, max)
		}
		prev = got
	}

	if got := retryDelay(4, base, max); got != max {
		t.Errorf("retryDelay(4) = %v, want the cap %v", got, max)
	}
	if got := retryDelay(1<<30, base, max); got != max {
		t.Errorf("retryDelay(1<<30) = %v, want the cap %v (large n must not overflow)", got, max)
	}
}
