package metrics

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"horizonx/internal/domain"

	"github.com/google/uuid"
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
}
