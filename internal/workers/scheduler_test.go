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

// 1. Not acquired: the worker must never run and release must never be called.
func TestSchedulerDoesNotRunWorkerWhenLockNotAcquired(t *testing.T) {
	locker := &fakeLocker{acquired: false}
	worker := &fakeWorker{name: "metrics_collect"}
	s := NewScheduler(nil, noopLog{}, locker)

	s.runWorker(context.Background(), worker)

	if worker.runs != 0 {
		t.Errorf("worker ran %d times, want 0 when the lock was not acquired", worker.runs)
	}
	if got := locker.releaseCount(); got != 0 {
		t.Errorf("release called %d times, want 0 when the lock was not acquired", got)
	}
}

// 2. Acquired: the worker runs and the deferred release fires afterwards.
func TestSchedulerRunsWorkerAndReleasesWhenLockAcquired(t *testing.T) {
	locker := &fakeLocker{acquired: true}
	worker := &fakeWorker{name: "job_reaper"}
	s := NewScheduler(nil, noopLog{}, locker)

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
	s := NewScheduler(nil, noopLog{}, locker)

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
	s := NewScheduler(nil, noopLog{}, locker)

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
	s := NewScheduler(nil, noopLog{}, nil)

	s.runWorker(context.Background(), worker)

	if worker.runs != 1 {
		t.Errorf("worker ran %d times, want 1 with a nil locker", worker.runs)
	}
}

// 6. The locker is asked about the worker's own name.
func TestSchedulerPassesWorkerNameToLocker(t *testing.T) {
	locker := &fakeLocker{acquired: true}
	worker := &fakeWorker{name: "application_health_check"}
	s := NewScheduler(nil, noopLog{}, locker)

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
	s := NewScheduler(nil, noopLog{}, locker)

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
