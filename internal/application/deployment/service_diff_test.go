package deployment

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"horizonx/internal/domain"
)

type fakeDeploymentRepo struct {
	deployments map[int64]*domain.Deployment
}

func (f *fakeDeploymentRepo) List(ctx context.Context, opts domain.DeploymentListOptions) ([]*domain.Deployment, int64, error) {
	return nil, 0, nil
}
func (f *fakeDeploymentRepo) GetByID(ctx context.Context, id int64) (*domain.Deployment, error) {
	d, ok := f.deployments[id]
	if !ok {
		return nil, domain.ErrDeploymentNotFound
	}
	return d, nil
}
func (f *fakeDeploymentRepo) Create(ctx context.Context, d *domain.Deployment) (*domain.Deployment, error) { return d, nil }
func (f *fakeDeploymentRepo) Start(ctx context.Context, id int64) (*domain.Deployment, error)              { return nil, nil }
func (f *fakeDeploymentRepo) Finish(ctx context.Context, id int64) (*domain.Deployment, error)             { return nil, nil }
func (f *fakeDeploymentRepo) UpdateStatus(ctx context.Context, id int64, s domain.DeploymentStatus) (*domain.Deployment, error) {
	return nil, nil
}
func (f *fakeDeploymentRepo) UpdateCommitInfo(ctx context.Context, id int64, hash, msg string) (*domain.Deployment, error) {
	return nil, nil
}
func (f *fakeDeploymentRepo) UpdateEnvSnapshot(ctx context.Context, id int64, snapshot map[string]string) (*domain.Deployment, error) {
	raw, _ := json.Marshal(snapshot)
	if d, ok := f.deployments[id]; ok {
		d.EnvSnapshot = raw
	}
	return nil, nil
}

func snap(m map[string]string) json.RawMessage {
	b, _ := json.Marshal(m)
	return b
}

func TestDiffNoPrevious(t *testing.T) {
	svc := &Service{repo: &fakeDeploymentRepo{deployments: map[int64]*domain.Deployment{
		1: {ID: 1, EnvSnapshot: snap(map[string]string{"A": "1", "B": "2"})},
	}}}

	diff, err := svc.Diff(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if diff.HasPrevious {
		t.Fatal("expected HasPrevious=false for first deployment")
	}
	if len(diff.EnvAdditions) != 2 || len(diff.EnvUpdates) != 0 || len(diff.EnvRemovals) != 0 {
		t.Fatalf("unexpected diff: %+v", diff)
	}
}

func TestDiffAddUpdateRemove(t *testing.T) {
	prevID := int64(1)
	svc := &Service{repo: &fakeDeploymentRepo{deployments: map[int64]*domain.Deployment{
		1: {ID: 1, EnvSnapshot: snap(map[string]string{"A": "old", "B": "keep", "C": "gone"}), CommitHash: strPtr("def456")},
		2: {
			ID: 2, EnvSnapshot: snap(map[string]string{"A": "new", "B": "keep", "D": "added"}),
			PreviousDeploymentID: &prevID,
			CommitHash:           strPtr("abc123"),
		},
	}}}

	diff, err := svc.Diff(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if !diff.HasPrevious {
		t.Fatal("expected HasPrevious=true")
	}
	if diff.CommitFrom == nil || *diff.CommitFrom != "def456" {
		t.Fatalf("unexpected commit_from: %v", diff.CommitFrom)
	}
	if len(diff.EnvAdditions) != 1 || diff.EnvAdditions[0].Key != "D" {
		t.Fatalf("unexpected additions: %+v", diff.EnvAdditions)
	}
	if len(diff.EnvUpdates) != 1 || diff.EnvUpdates[0].Key != "A" || diff.EnvUpdates[0].Old != "old" || diff.EnvUpdates[0].New != "new" {
		t.Fatalf("unexpected updates: %+v", diff.EnvUpdates)
	}
	if len(diff.EnvRemovals) != 1 || diff.EnvRemovals[0].Key != "C" {
		t.Fatalf("unexpected removals: %+v", diff.EnvRemovals)
	}
}

func TestDiffPreviousGoneDegrades(t *testing.T) {
	prevID := int64(99) // doesn't exist
	svc := &Service{repo: &fakeDeploymentRepo{deployments: map[int64]*domain.Deployment{
		2: {ID: 2, EnvSnapshot: snap(map[string]string{"A": "1"}), PreviousDeploymentID: &prevID},
	}}}

	diff, err := svc.Diff(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if diff.HasPrevious {
		t.Fatal("expected HasPrevious=false when previous deployment is missing")
	}
	if len(diff.EnvAdditions) != 1 {
		t.Fatalf("expected all env vars as additions, got %+v", diff.EnvAdditions)
	}
}

func TestDiffNotFound(t *testing.T) {
	svc := &Service{repo: &fakeDeploymentRepo{deployments: map[int64]*domain.Deployment{}}}
	_, err := svc.Diff(context.Background(), 42)
	if !errors.Is(err, domain.ErrDeploymentNotFound) {
		t.Fatalf("expected ErrDeploymentNotFound, got %v", err)
	}
}

func strPtr(s string) *string { return &s }
