package job

import (
	"context"
	"testing"

	"horizonx/internal/domain"
	"horizonx/internal/event"

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
	return f.counts, nil
}

func TestServiceSummaryReturnsCounts(t *testing.T) {
	svc := NewService(&fakeJobRepo{
		counts: &domain.JobStatusCounts{Queued: 4, Running: 2, Success: 10, Failed: 1, Total: 17},
	}, nil, event.New())

	counts, err := svc.Summary(context.Background())
	if err != nil {
		t.Fatalf("Summary returned error: %v", err)
	}
	if counts.Queued != 4 || counts.Running != 2 || counts.Success != 10 || counts.Failed != 1 || counts.Total != 17 {
		t.Fatalf("unexpected counts: %+v", counts)
	}
}
