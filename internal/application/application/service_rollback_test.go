package application_test

import (
	"context"
	"testing"

	"horizonx/internal/application/application"
	"horizonx/internal/domain"
	"horizonx/internal/mocks"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// fakeDeploymentSvc implements domain.DeploymentService for rollback tests.
type fakeDeploymentSvc struct {
	prev []*domain.Deployment
}

func (f *fakeDeploymentSvc) List(_ context.Context, _ domain.DeploymentListOptions) (*domain.ListResult[*domain.Deployment], error) {
	return &domain.ListResult[*domain.Deployment]{Data: f.prev}, nil
}

func (f *fakeDeploymentSvc) GetByID(context.Context, int64) (*domain.Deployment, error) {
	return nil, nil
}

func (f *fakeDeploymentSvc) Create(context.Context, domain.DeploymentCreateRequest) (*domain.Deployment, error) {
	return &domain.Deployment{ID: 99, Status: domain.DeploymentPending}, nil
}

func (f *fakeDeploymentSvc) Start(context.Context, int64) error                       { return nil }
func (f *fakeDeploymentSvc) Finish(context.Context, int64) error                      { return nil }
func (f *fakeDeploymentSvc) UpdateStatus(context.Context, int64, domain.DeploymentStatus) error {
	return nil
}

func (f *fakeDeploymentSvc) UpdateCommitInfo(context.Context, int64, string, string) error {
	return nil
}

func commitPtr(s string) *string { return &s }

// fakeJobSvc implements domain.JobService for rollback tests.
type fakeJobSvc struct {
	created []*domain.Job
}

func (f *fakeJobSvc) List(context.Context, domain.JobListOptions) (*domain.ListResult[*domain.Job], error) {
	return &domain.ListResult[*domain.Job]{}, nil
}

func (f *fakeJobSvc) GetPending(context.Context, uuid.UUID) ([]*domain.Job, error) { return nil, nil }
func (f *fakeJobSvc) GetByID(context.Context, int64) (*domain.Job, error)          { return nil, nil }

func (f *fakeJobSvc) Create(_ context.Context, j *domain.Job) (*domain.Job, error) {
	f.created = append(f.created, j)
	return &domain.Job{ID: 55}, nil
}

func (f *fakeJobSvc) Delete(context.Context, int64) error                 { return nil }
func (f *fakeJobSvc) Retry(context.Context, int64, *domain.Job) (*domain.Job, error) {
	return nil, nil
}
func (f *fakeJobSvc) Start(context.Context, int64) (*domain.Job, error)             { return nil, nil }
func (f *fakeJobSvc) Finish(context.Context, int64, domain.JobStatus) (*domain.Job, error) {
	return nil, nil
}
func (f *fakeJobSvc) Summary(context.Context) (*domain.JobStatusCounts, error) {
	return &domain.JobStatusCounts{}, nil
}


// P0-4: Rollback creates a job pointing at the last successful deployment's
// image tag (<appKey>:<commitHash>).
func TestRollbackUsesLastSuccessfulCommit(t *testing.T) {
	appRepo := mocks.NewMockApplicationRepository(t)

	serverID := uuid.New()
	appRepo.EXPECT().GetByID(mock.Anything, int64(1)).Return(&domain.Application{
		ID:       1,
		ServerID: serverID,
		RepoName: "demo-app",
		Branch:   "main",
		Status:   domain.AppStatusRunning,
	}, nil)

	appRepo.EXPECT().ListEnvVars(mock.Anything, int64(1)).Return([]domain.EnvironmentVariable{
		{Key: "FOO", Value: "bar"},
	}, nil)

	jobSvc := &fakeJobSvc{}

	deploySvc := &fakeDeploymentSvc{prev: []*domain.Deployment{
		{ID: 3, Status: domain.DeploymentSuccess, CommitHash: commitPtr("0123456789abcdef0123456789abcdef01234567")},
	}}

	svc := application.NewService(appRepo, nil, jobSvc, deploySvc, nil)

	dep, err := svc.Rollback(context.Background(), 1, 1)
	assert.NoError(t, err)
	assert.NotNil(t, dep)
	assert.Len(t, jobSvc.created, 1)
	assert.Equal(t, domain.JobTypeAppRollback, jobSvc.created[0].Type)
}

// P0-4: rollback with no successful deployment must fail cleanly.
func TestRollbackFailsWithoutSuccessfulDeployment(t *testing.T) {
	appRepo := mocks.NewMockApplicationRepository(t)

	appRepo.EXPECT().GetByID(mock.Anything, int64(1)).Return(&domain.Application{
		ID:       1,
		ServerID: uuid.New(),
		RepoName: "demo-app",
		Branch:   "main",
	}, nil)

	svc := application.NewService(appRepo, nil, &fakeJobSvc{}, &fakeDeploymentSvc{}, nil)

	_, err := svc.Rollback(context.Background(), 1, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no successful deployment")
}
