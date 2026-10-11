package metrics

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"horizonx/internal/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type fakeJobRepo struct {
	counts *domain.JobStatusCounts
}

func (f *fakeJobRepo) List(ctx context.Context, opts domain.JobListOptions) ([]*domain.Job, int64, error) {
	return nil, 0, nil
}
func (f *fakeJobRepo) GetPending(ctx context.Context, serverID uuid.UUID) ([]*domain.Job, error) {
	return nil, nil
}
func (f *fakeJobRepo) GetByID(ctx context.Context, jobID int64) (*domain.Job, error) {
	return nil, nil
}
func (f *fakeJobRepo) Create(ctx context.Context, j *domain.Job) (*domain.Job, error) {
	return nil, nil
}
func (f *fakeJobRepo) Delete(ctx context.Context, jobID int64) error {
	return nil
}
func (f *fakeJobRepo) Retry(ctx context.Context, jobID int64, j *domain.Job) (*domain.Job, error) {
	return nil, nil
}
func (f *fakeJobRepo) MarkRunning(ctx context.Context, jobID int64) (*domain.Job, error) {
	return nil, nil
}
func (f *fakeJobRepo) MarkFinished(ctx context.Context, jobID int64, status domain.JobStatus) (*domain.Job, error) {
	return nil, nil
}
func (f *fakeJobRepo) CountsByStatus(ctx context.Context) (*domain.JobStatusCounts, error) {
	if f.counts != nil {
		return f.counts, nil
	}
	return &domain.JobStatusCounts{Queued: 3, Running: 1, Total: 4}, nil
}

type fakeServerRepo struct {
	online int64
}

func (f *fakeServerRepo) List(ctx context.Context, opts domain.ServerListOptions) ([]*domain.Server, int64, error) {
	return nil, 0, nil
}
func (f *fakeServerRepo) GetByID(ctx context.Context, serverID uuid.UUID) (*domain.Server, error) {
	return nil, nil
}
func (f *fakeServerRepo) GetByToken(ctx context.Context, token string) (*domain.Server, error) {
	return nil, nil
}
func (f *fakeServerRepo) Create(ctx context.Context, s *domain.Server) (*domain.Server, error) {
	return nil, nil
}
func (f *fakeServerRepo) Update(ctx context.Context, s *domain.Server, serverID uuid.UUID) error {
	return nil
}
func (f *fakeServerRepo) UpdateOSInfo(ctx context.Context, serverID uuid.UUID, osInfo domain.OSInfo) error {
	return nil
}
func (f *fakeServerRepo) UpdateStatus(ctx context.Context, serverID uuid.UUID, isOnline bool) error {
	return nil
}
func (f *fakeServerRepo) UpdateSecret(ctx context.Context, serverID uuid.UUID, secret string) error {
	return nil
}
func (f *fakeServerRepo) Delete(ctx context.Context, serverID uuid.UUID) error {
	return nil
}
func (f *fakeServerRepo) CountOnline(ctx context.Context) (int64, error) {
	return f.online, nil
}

func TestRegistryExposesMetrics(t *testing.T) {
	reg := NewRegistry(&fakeJobRepo{}, &fakeServerRepo{online: 1}, nil)
	reg.ObserveRequest("GET", "/health", "OK", 0.01)

	reg.Refresh()
	rec := httptest.NewRecorder()
	reg.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	body, _ := io.ReadAll(rec.Result().Body)
	out := string(body)

	for _, want := range []string{
		"horizonx_http_requests_total",
		"horizonx_http_request_duration_seconds",
		"horizonx_jobs_pending 3",
		"horizonx_jobs_running 1",
		"horizonx_servers_online 1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in metrics output", want)
		}
	}
	reg.Stop()
}

// countingJobRepo wraps fakeJobRepo and counts synchronous DB hits.
type countingJobRepo struct {
	*fakeJobRepo
	calls atomic.Int64
}

func (c *countingJobRepo) CountsByStatus(ctx context.Context) (*domain.JobStatusCounts, error) {
	c.calls.Add(1)
	return c.fakeJobRepo.CountsByStatus(ctx)
}

// blockingJobRepo blocks inside CountsByStatus until released, so a slow
// refresh can be held open while a second tick fires.
type blockingJobRepo struct {
	countingJobRepo
	release chan struct{}
	entered chan struct{}
	once    sync.Once
}

func (b *blockingJobRepo) CountsByStatus(ctx context.Context) (*domain.JobStatusCounts, error) {
	b.calls.Add(1)
	b.once.Do(func() { close(b.entered) })
	<-b.release
	return &domain.JobStatusCounts{}, nil
}

type errJobRepo struct {
	fakeJobRepo
}

func (e *errJobRepo) CountsByStatus(ctx context.Context) (*domain.JobStatusCounts, error) {
	return nil, errors.New("boom")
}

func TestScrapeServesCachedValuesWithoutSyncRefresh(t *testing.T) {
	jobs := &countingJobRepo{fakeJobRepo: &fakeJobRepo{}}
	reg := NewRegistry(jobs, &fakeServerRepo{online: 1}, nil)
	defer reg.Stop()

	// Fresh registry has not refreshed yet: gauges stay zero and the
	// handler never triggers a synchronous DB query.
	require.False(t, reg.Refreshed())
	require.Zero(t, jobs.calls.Load())

	rec := httptest.NewRecorder()
	reg.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Zero(t, jobs.calls.Load(), "scrape must not run a synchronous refresh")

	body, _ := io.ReadAll(rec.Result().Body)
	require.Contains(t, string(body), "horizonx_jobs_pending 0")
}

// A failing refresh must not flip gauges to garbage, must still mark the
// registry refreshed, and must not stop the background loop.
func TestRefreshErrorIsLoggedAndLoopSurvives(t *testing.T) {
	reg := NewRegistry(&errJobRepo{}, &fakeServerRepo{online: 2}, nil)
	defer reg.Stop()

	reg.Refresh()
	require.True(t, reg.Refreshed())

	rec := httptest.NewRecorder()
	reg.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body, _ := io.ReadAll(rec.Result().Body)
	require.Contains(t, string(body), "horizonx_servers_online 2")
}

// A second refresh tick that fires while a slow refresh is still running
// must be skipped, not stacked.
func TestOverlappingRefreshesDoNotStack(t *testing.T) {
	jobs := &blockingJobRepo{release: make(chan struct{}), entered: make(chan struct{})}
	jobs.fakeJobRepo = &fakeJobRepo{}
	reg := NewRegistry(jobs, &fakeServerRepo{}, nil)
	defer reg.Stop()

	done := make(chan struct{})
	go func() { reg.Refresh(); close(done) }()

	select {
	case <-jobs.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("slow refresh never started")
	}

	// While the slow refresh holds the guard, this call must return
	// immediately without touching the repo again.
	reg.Refresh()
	require.Equal(t, int64(1), jobs.calls.Load())

	close(jobs.release)
	<-done
	require.True(t, reg.Refreshed())
}

// The background loop must stop on cancellation — no ticks after cancel.
func TestBackgroundLoopStopsOnCancel(t *testing.T) {
	jobs := &countingJobRepo{fakeJobRepo: &fakeJobRepo{}}
	reg := NewRegistry(jobs, &fakeServerRepo{}, nil)
	defer reg.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { reg.loop(ctx, 10*time.Millisecond); close(done) }()

	// Let at least one tick fire, then cancel and confirm the loop exits
	// promptly and performs no further refreshes.
	require.Eventually(t, func() bool { return jobs.calls.Load() > 0 }, 5*time.Second, time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("loop did not stop after cancel")
	}
	n := jobs.calls.Load()
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, n, jobs.calls.Load(), "no refreshes after cancel")
}

func TestAgentDroppedMessagesGaugeIsSetAndExposed(t *testing.T) {
	reg := NewRegistry(&fakeJobRepo{}, &fakeServerRepo{}, nil)
	defer reg.Stop()

	reg.SetAgentDroppedMessages("server-a", 5)
	reg.SetAgentDroppedMessages("server-b", 12)

	require.Equal(t, float64(5), reg.AgentDroppedMessagesValue("server-a"))
	require.Equal(t, float64(12), reg.AgentDroppedMessagesValue("server-b"))

	rec := httptest.NewRecorder()
	reg.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	body, _ := io.ReadAll(rec.Result().Body)
	out := string(body)
	require.Contains(t, out, "horizonx_agent_dropped_messages")
	require.Contains(t, out, `server_id="server-a"`)
	require.Contains(t, out, `server_id="server-b"`)
}
