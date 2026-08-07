package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"horizonx/internal/adapters/http/request"
	"horizonx/internal/adapters/http/response"
	"horizonx/internal/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeJobService is an in-memory JobService for handler tests.
type fakeJobService struct {
	jobs map[int64]*domain.Job

	retryCalls int
}

func newFakeJobService() *fakeJobService {
	return &fakeJobService{jobs: map[int64]*domain.Job{}}
}

func (f *fakeJobService) List(ctx context.Context, opts domain.JobListOptions) (*domain.ListResult[*domain.Job], error) {
	return nil, nil
}

func (f *fakeJobService) GetPending(ctx context.Context, serverID uuid.UUID) ([]*domain.Job, error) {
	return nil, nil
}

func (f *fakeJobService) GetByID(ctx context.Context, jobID int64) (*domain.Job, error) {
	job, ok := f.jobs[jobID]
	if !ok {
		return nil, domain.ErrJobNotFound
	}
	return job, nil
}

func (f *fakeJobService) Create(ctx context.Context, j *domain.Job) (*domain.Job, error) {
	return nil, nil
}

func (f *fakeJobService) Delete(ctx context.Context, jobID int64) error {
	return nil
}

func (f *fakeJobService) Retry(ctx context.Context, jobID int64, j *domain.Job) (*domain.Job, error) {
	f.retryCalls++
	job, ok := f.jobs[jobID]
	if !ok {
		return nil, domain.ErrJobNotFound
	}
	job.Status = j.Status
	job.QueuedAt = j.QueuedAt
	job.Payload = j.Payload
	return job, nil
}

func (f *fakeJobService) Start(ctx context.Context, jobID int64) (*domain.Job, error) {
	return nil, nil
}

func (f *fakeJobService) Finish(ctx context.Context, jobID int64, status domain.JobStatus) (*domain.Job, error) {
	return nil, nil
}

func (f *fakeJobService) Summary(ctx context.Context) (*domain.JobStatusCounts, error) {
	return nil, nil
}

func newJobTestHandler(svc domain.JobService) *JobHandler {
	return NewJobHandler(
		svc,
		request.NewJSONDecoder(),
		response.NewJSONWriter(stubLogger{}),
		nil,
	)
}

func TestJobHandler_Retry_Success(t *testing.T) {
	svc := newFakeJobService()
	now := time.Now().UTC()
	svc.jobs[42] = &domain.Job{
		ID:        42,
		Type:      domain.JobTypeAppDeploy,
		Status:    domain.JobFailed,
		Payload:   []byte(`{"app_id":7}`),
		QueuedAt:  &now,
		ExpiredAt: &now,
	}

	h := newJobTestHandler(svc)
	req := httptest.NewRequest(http.MethodPost, "/jobs/42/retry", nil)
	req.SetPathValue("id", "42")
	rec := httptest.NewRecorder()

	h.Retry(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, svc.retryCalls)

	job := svc.jobs[42]
	assert.Equal(t, domain.JobQueued, job.Status)
	assert.NotNil(t, job.QueuedAt)
	assert.Equal(t, json.RawMessage(`{"app_id":7}`), job.Payload)
}

func TestJobHandler_Retry_NotFound(t *testing.T) {
	svc := newFakeJobService()
	h := newJobTestHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/jobs/99/retry", nil)
	req.SetPathValue("id", "99")
	rec := httptest.NewRecorder()

	h.Retry(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, 0, svc.retryCalls)
}

func TestJobHandler_Retry_NotRetryable(t *testing.T) {
	svc := newFakeJobService()
	now := time.Now().UTC()
	svc.jobs[7] = &domain.Job{
		ID:       7,
		Type:     domain.JobTypeAppDeploy,
		Status:   domain.JobRunning,
		QueuedAt: &now,
	}

	h := newJobTestHandler(svc)
	req := httptest.NewRequest(http.MethodPost, "/jobs/7/retry", nil)
	req.SetPathValue("id", "7")
	rec := httptest.NewRecorder()

	h.Retry(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Equal(t, 0, svc.retryCalls)
}

func TestJobHandler_Retry_InvalidID(t *testing.T) {
	svc := newFakeJobService()
	h := newJobTestHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/jobs/abc/retry", nil)
	req.SetPathValue("id", "abc")
	rec := httptest.NewRecorder()

	h.Retry(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, 0, svc.retryCalls)
}
