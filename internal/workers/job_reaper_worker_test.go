package workers

import (
	"context"
	"testing"
	"time"

	"horizonx/internal/agent"
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

func (f *fakeJobService) Summary(context.Context) (*domain.JobStatusCounts, error) {
	return &domain.JobStatusCounts{}, nil
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

// TestReapAfterOutlastsAgentJobTimeouts guards a cross-package invariant that
// nothing else connects: reapAfter lives here, the agent's job timeouts live in
// internal/agent, and the two constants are only correct relative to each
// other. Both clocks start from the same event — the agent writing StartedAt —
// so a job type added later that runs longer than reapAfter makes the reaper
// mark live jobs failed while the agent is still working on them.
//
// The spec filed this as the opposite problem ("a window where a job is reaped
// while the agent may still be working") and proposed aligning the two
// timeouts, which would have removed the existing margin rather than protected
// it. The guard is the part of that finding worth keeping.
func TestReapAfterOutlastsAgentJobTimeouts(t *testing.T) {
	longest := agent.MaxJobTimeout()

	if reapAfter <= longest {
		t.Fatalf("reapAfter (%s) must exceed the longest agent job timeout (%s), "+
			"otherwise a job the agent is still running gets reaped; raise reapAfter "+
			"or lower the job timeout", reapAfter, longest)
	}

	// The 2x margin is deliberate, not incidental: the agent gives up at
	// `longest`, but it still has to notice that, report it over the websocket,
	// and have the server persist it before the reaper would race it.
	if want := 2 * longest; reapAfter < want {
		t.Errorf("margin too thin: reapAfter = %s, want at least %s (2x the longest "+
			"agent job timeout of %s)", reapAfter, want, longest)
	}
}
