package workers

import (
	"context"
	"testing"
	"time"

	"horizonx/internal/domain"

	"github.com/google/uuid"
)

type fakeJobService struct {
	jobs      []*domain.Job
	finished  []int64
	failError error
}

func (f *fakeJobService) List(_ context.Context, opts domain.JobListOptions) (*domain.ListResult[*domain.Job], error) {
	return &domain.ListResult[*domain.Job]{Data: f.jobs}, nil
}

func (f *fakeJobService) GetPending(context.Context, uuid.UUID) ([]*domain.Job, error) {
	return nil, nil
}
func (f *fakeJobService) GetByID(context.Context, int64) (*domain.Job, error) { return nil, nil }
func (f *fakeJobService) Create(context.Context, *domain.Job) (*domain.Job, error) {
	return nil, nil
}
func (f *fakeJobService) Delete(context.Context, int64) error                 { return nil }
func (f *fakeJobService) Retry(context.Context, int64, *domain.Job) (*domain.Job, error) {
	return nil, nil
}
func (f *fakeJobService) Start(context.Context, int64) (*domain.Job, error) { return nil, nil }

func (f *fakeJobService) Finish(_ context.Context, jobID int64, _ domain.JobStatus) (*domain.Job, error) {
	if f.failError != nil {
		return nil, f.failError
	}
	f.finished = append(f.finished, jobID)
	return nil, nil
}

func jobPtr(id int64, startedAt *time.Time) *domain.Job {
	return &domain.Job{ID: id, Type: domain.JobTypeAppDeploy, StartedAt: startedAt}
}

func TestJobReaperReapsStuckJobs(t *testing.T) {
	old := time.Now().Add(-reapAfter - time.Minute)
	recent := time.Now().Add(-1 * time.Minute)

	svc := &fakeJobService{jobs: []*domain.Job{
		jobPtr(1, &old),    // stuck -> should be reaped
		jobPtr(2, &recent), // recent -> keep
		jobPtr(3, nil),     // no start time -> keep
	}}

	w := NewJobReaperWorker(svc, noopLog{})
	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("reaper run failed: %v", err)
	}

	if len(svc.finished) != 1 || svc.finished[0] != 1 {
		t.Fatalf("expected only job 1 reaped, got %v", svc.finished)
	}
}

func TestJobReaperSurvivesErrors(t *testing.T) {
	old := time.Now().Add(-reapAfter - time.Minute)
	svc := &fakeJobService{
		jobs:      []*domain.Job{jobPtr(1, &old), jobPtr(2, &old)},
		failError: context.DeadlineExceeded,
	}

	w := NewJobReaperWorker(svc, noopLog{})
	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("reaper must not return error when finish fails: %v", err)
	}
}

// noopLog satisfies logger.Logger without producing output.
type noopLog struct{}

func (noopLog) Debug(string, ...any) {}
func (noopLog) Info(string, ...any)  {}
func (noopLog) Warn(string, ...any)  {}
func (noopLog) Error(string, ...any) {}
