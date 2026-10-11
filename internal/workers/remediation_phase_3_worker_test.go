package workers

import (
	"context"
	"testing"

	"horizonx/internal/domain"

	"github.com/google/uuid"
)

// recordingJobService is a JobService fake whose List honours the Statuses,
// ServerID, Type and Limit options the way the real postgres repository does,
// and which records every call. Honour the filter is what makes these tests
// exercise the real defect: a fake that ignored Statuses would return a
// finished job for an open-only query and hide the bug.
type recordingJobService struct {
	jobs     []*domain.Job
	listOpts []domain.JobListOptions
	created  int
	retried  int
}

func (f *recordingJobService) List(_ context.Context, opts domain.JobListOptions) (*domain.ListResult[*domain.Job], error) {
	f.listOpts = append(f.listOpts, opts)

	statuses := make(map[domain.JobStatus]bool, len(opts.Statuses))
	for _, s := range opts.Statuses {
		statuses[domain.JobStatus(s)] = true
	}

	out := []*domain.Job{}
	for _, j := range f.jobs {
		if opts.ServerID != nil && j.ServerID != *opts.ServerID {
			continue
		}
		if opts.Type != "" && string(j.Type) != opts.Type {
			continue
		}
		if len(statuses) > 0 && !statuses[j.Status] {
			continue
		}
		out = append(out, j)
		if opts.Limit > 0 && len(out) >= opts.Limit {
			break
		}
	}

	return &domain.ListResult[*domain.Job]{Data: out}, nil
}

func (f *recordingJobService) GetPending(context.Context, uuid.UUID) ([]*domain.Job, error) {
	return nil, nil
}

func (f *recordingJobService) GetByID(context.Context, int64) (*domain.Job, error) { return nil, nil }

func (f *recordingJobService) Create(_ context.Context, j *domain.Job) (*domain.Job, error) {
	f.created++
	j.ID = int64(f.created)
	return j, nil
}

func (f *recordingJobService) Delete(context.Context, int64) error { return nil }

func (f *recordingJobService) Retry(_ context.Context, _ int64, j *domain.Job) (*domain.Job, error) {
	f.retried++
	return j, nil
}

func (f *recordingJobService) Start(context.Context, int64) (*domain.Job, error) { return nil, nil }

func (f *recordingJobService) Finish(context.Context, int64, domain.JobStatus) (*domain.Job, error) {
	return nil, nil
}

func (f *recordingJobService) Summary(context.Context) (*domain.JobStatusCounts, error) {
	return &domain.JobStatusCounts{}, nil
}

// openStatuses is the exact filter both workers must pass: the two states in
// which a job is still scheduled to run. A finished job must never match.
var openStatuses = []string{string(domain.JobQueued), string(domain.JobRunning)}

func assertOpenStatusesFilter(t *testing.T, f *recordingJobService) {
	t.Helper()
	if len(f.listOpts) == 0 {
		t.Fatal("worker never called List")
	}
	got := f.listOpts[0].Statuses
	if len(got) != 2 {
		t.Fatalf("expected exactly the open statuses %v, got %v", openStatuses, got)
	}
	seen := map[string]bool{}
	for _, s := range got {
		seen[s] = true
	}
	for _, want := range openStatuses {
		if !seen[want] {
			t.Fatalf("List Statuses missing %q: got %v", want, got)
		}
	}
}

func assertCalls(t *testing.T, f *recordingJobService, wantCreated, wantRetried int) {
	t.Helper()
	if f.created != wantCreated {
		t.Fatalf("Create calls = %d, want %d", f.created, wantCreated)
	}
	if f.retried != wantRetried {
		t.Fatalf("Retry calls = %d, want %d", f.retried, wantRetried)
	}
}

// fakeServerService satisfies domain.ServerService for the metrics worker.
type fakeServerService struct {
	servers []*domain.Server
}

func (f *fakeServerService) List(context.Context, domain.ServerListOptions) (*domain.ListResult[*domain.Server], error) {
	return &domain.ListResult[*domain.Server]{Data: f.servers}, nil
}

func (f *fakeServerService) GetByID(context.Context, uuid.UUID) (*domain.Server, error) {
	return nil, nil
}

func (f *fakeServerService) Register(context.Context, domain.ServerSaveRequest) (*domain.Server, string, error) {
	return nil, "", nil
}

func (f *fakeServerService) Update(context.Context, domain.ServerSaveRequest, uuid.UUID) error {
	return nil
}

func (f *fakeServerService) UpdateOSInfo(context.Context, uuid.UUID, domain.OSInfo) error { return nil }
func (f *fakeServerService) UpdateStatus(context.Context, uuid.UUID, bool) error          { return nil }
func (f *fakeServerService) Delete(context.Context, uuid.UUID) error                      { return nil }

func (f *fakeServerService) AuthorizeAgent(context.Context, uuid.UUID, string) (*domain.Server, error) {
	return nil, nil
}

func (f *fakeServerService) RotateSecret(context.Context, uuid.UUID) (string, error) { return "", nil }

// fakeApplicationService satisfies domain.ApplicationService for the health
// check worker; only List is exercised by the worker.
type fakeApplicationService struct {
	apps []*domain.Application
}

func (f *fakeApplicationService) List(context.Context, domain.ApplicationListOptions) (*domain.ListResult[*domain.Application], error) {
	return &domain.ListResult[*domain.Application]{Data: f.apps}, nil
}

func (f *fakeApplicationService) GetByID(context.Context, int64) (*domain.Application, error) {
	return nil, nil
}

func (f *fakeApplicationService) Create(context.Context, domain.ApplicationCreateRequest) (*domain.Application, error) {
	return nil, nil
}

func (f *fakeApplicationService) Update(context.Context, domain.ApplicationUpdateRequest, int64) error {
	return nil
}

func (f *fakeApplicationService) UpdateStatus(context.Context, int64, domain.ApplicationStatus) error {
	return nil
}

func (f *fakeApplicationService) UpdateLastDeployment(context.Context, int64) error { return nil }

func (f *fakeApplicationService) UpdateHealth(context.Context, uuid.UUID, []domain.ApplicationHealth) error {
	return nil
}

func (f *fakeApplicationService) Delete(context.Context, int64) error { return nil }

func (f *fakeApplicationService) Deploy(context.Context, int64, int64) (*domain.Deployment, error) {
	return nil, nil
}

func (f *fakeApplicationService) Rollback(context.Context, int64, int64) (*domain.Deployment, error) {
	return nil, nil
}

func (f *fakeApplicationService) Start(context.Context, int64) error   { return nil }
func (f *fakeApplicationService) Stop(context.Context, int64) error    { return nil }
func (f *fakeApplicationService) Restart(context.Context, int64) error { return nil }

func (f *fakeApplicationService) ListEnvVars(context.Context, int64) ([]domain.EnvironmentVariable, error) {
	return nil, nil
}

func (f *fakeApplicationService) AddEnvVar(context.Context, int64, domain.EnvironmentVariableRequest) error {
	return nil
}

func (f *fakeApplicationService) UpdateEnvVar(context.Context, int64, string, domain.EnvironmentVariableRequest) error {
	return nil
}

func (f *fakeApplicationService) DeleteEnvVar(context.Context, int64, string) error { return nil }

func newMetricsWorkerFixture(t *testing.T, svc *recordingJobService) (*MetricsCollectWorker, uuid.UUID) {
	t.Helper()
	serverID := uuid.New()
	srvSvc := &fakeServerService{servers: []*domain.Server{{ID: serverID, IsOnline: true}}}
	w := NewMetricsCollectWorker(svc, srvSvc, noopLog{}).(*MetricsCollectWorker)
	return w, serverID
}

func metricsJob(serverID uuid.UUID, status domain.JobStatus) *domain.Job {
	return &domain.Job{ID: 1, ServerID: serverID, Type: domain.JobTypeMetricsCollect, Status: status}
}

// TestMetricsWorkerDoesNotResurrectFinishedJob is the regression test for the
// infinite requeue loop: a finished job is history, so the worker must create a
// fresh job and never call Retry on the finished row.
func TestMetricsWorkerDoesNotResurrectFinishedJob(t *testing.T) {
	svc := &recordingJobService{jobs: []*domain.Job{metricsJob(uuid.Nil, domain.JobSuccess)}}
	w, serverID := newMetricsWorkerFixture(t, svc)
	svc.jobs[0].ServerID = serverID

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	assertCalls(t, svc, 1, 0)
	assertOpenStatusesFilter(t, svc)
}

// TestMetricsWorkerSkipsWhenQueuedJobOpen pins that an already-scheduled job is
// left alone: neither a new row nor a requeue is warranted.
func TestMetricsWorkerSkipsWhenQueuedJobOpen(t *testing.T) {
	svc := &recordingJobService{jobs: []*domain.Job{metricsJob(uuid.Nil, domain.JobQueued)}}
	w, serverID := newMetricsWorkerFixture(t, svc)
	svc.jobs[0].ServerID = serverID

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	assertCalls(t, svc, 0, 0)
	assertOpenStatusesFilter(t, svc)
}

// TestMetricsWorkerSkipsWhenRunningJobOpen guards against resetting a job that
// the agent is currently executing back to queued.
func TestMetricsWorkerSkipsWhenRunningJobOpen(t *testing.T) {
	svc := &recordingJobService{jobs: []*domain.Job{metricsJob(uuid.Nil, domain.JobRunning)}}
	w, serverID := newMetricsWorkerFixture(t, svc)
	svc.jobs[0].ServerID = serverID

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	assertCalls(t, svc, 0, 0)
	assertOpenStatusesFilter(t, svc)
}

// TestMetricsWorkerCreatesWhenNoJobExists is the healthy path: an empty queue
// yields exactly one new job.
func TestMetricsWorkerCreatesWhenNoJobExists(t *testing.T) {
	svc := &recordingJobService{}
	w, _ := newMetricsWorkerFixture(t, svc)

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	assertCalls(t, svc, 1, 0)
	assertOpenStatusesFilter(t, svc)
}

func healthCheckJob(serverID uuid.UUID, status domain.JobStatus) *domain.Job {
	return &domain.Job{ID: 1, ServerID: serverID, Type: domain.JobTypeAppHealthCheck, Status: status}
}

func newHealthCheckWorkerFixture(t *testing.T, svc *recordingJobService) (*ApplicationHealthCheckWorker, uuid.UUID) {
	t.Helper()
	serverID := uuid.New()
	appSvc := &fakeApplicationService{apps: []*domain.Application{{
		ID:       1,
		ServerID: serverID,
		RepoName: "sample",
	}}}
	w := NewApplicationHealthCheckWorker(appSvc, svc, nil, noopLog{}).(*ApplicationHealthCheckWorker)
	return w, serverID
}

// TestHealthCheckWorkerDoesNotResurrectFinishedJob is the app-health-check twin
// of TestMetricsWorkerDoesNotResurrectFinishedJob.
func TestHealthCheckWorkerDoesNotResurrectFinishedJob(t *testing.T) {
	svc := &recordingJobService{jobs: []*domain.Job{healthCheckJob(uuid.Nil, domain.JobSuccess)}}
	w, serverID := newHealthCheckWorkerFixture(t, svc)
	svc.jobs[0].ServerID = serverID

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	assertCalls(t, svc, 1, 0)
	assertOpenStatusesFilter(t, svc)
}

// TestHealthCheckWorkerSkipsWhenQueuedJobOpen pins that an open job blocks a
// second one for the same server/type.
func TestHealthCheckWorkerSkipsWhenQueuedJobOpen(t *testing.T) {
	svc := &recordingJobService{jobs: []*domain.Job{healthCheckJob(uuid.Nil, domain.JobQueued)}}
	w, serverID := newHealthCheckWorkerFixture(t, svc)
	svc.jobs[0].ServerID = serverID

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	assertCalls(t, svc, 0, 0)
	assertOpenStatusesFilter(t, svc)
}
