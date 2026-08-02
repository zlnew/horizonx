package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"horizonx/internal/domain"

	"github.com/google/uuid"
)

type fakeAppSvc struct{}

func (f *fakeAppSvc) List(ctx context.Context, opts domain.ApplicationListOptions) (*domain.ListResult[*domain.Application], error) {
	return nil, nil
}
func (f *fakeAppSvc) GetByID(ctx context.Context, appID int64) (*domain.Application, error) {
	return &domain.Application{Name: "neo-portfolio"}, nil
}
func (f *fakeAppSvc) Create(ctx context.Context, req domain.ApplicationCreateRequest) (*domain.Application, error) {
	return nil, nil
}
func (f *fakeAppSvc) Update(ctx context.Context, req domain.ApplicationUpdateRequest, appID int64) error {
	return nil
}
func (f *fakeAppSvc) UpdateStatus(ctx context.Context, appID int64, status domain.ApplicationStatus) error {
	return nil
}
func (f *fakeAppSvc) UpdateLastDeployment(ctx context.Context, appID int64) error {
	return nil
}
func (f *fakeAppSvc) UpdateHealth(ctx context.Context, serverID uuid.UUID, reports []domain.ApplicationHealth) error {
	return nil
}
func (f *fakeAppSvc) Delete(ctx context.Context, appID int64) error {
	return nil
}
func (f *fakeAppSvc) Deploy(ctx context.Context, appID int64, deployedBy int64) (*domain.Deployment, error) {
	return nil, nil
}
func (f *fakeAppSvc) Rollback(ctx context.Context, appID int64, deployedBy int64) (*domain.Deployment, error) {
	return nil, nil
}
func (f *fakeAppSvc) Start(ctx context.Context, appID int64) error {
	return nil
}
func (f *fakeAppSvc) Stop(ctx context.Context, appID int64) error {
	return nil
}
func (f *fakeAppSvc) Restart(ctx context.Context, appID int64) error {
	return nil
}
func (f *fakeAppSvc) ListEnvVars(ctx context.Context, appID int64) ([]domain.EnvironmentVariable, error) {
	return nil, nil
}
func (f *fakeAppSvc) AddEnvVar(ctx context.Context, appID int64, req domain.EnvironmentVariableRequest) error {
	return nil
}
func (f *fakeAppSvc) UpdateEnvVar(ctx context.Context, appID int64, key string, req domain.EnvironmentVariableRequest) error {
	return nil
}
func (f *fakeAppSvc) DeleteEnvVar(ctx context.Context, appID int64, key string) error {
	return nil
}

type captureHandler struct {
	mu       sync.Mutex
	body     map[string]any
	requests int
}

func (h *captureHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.requests++
	defer r.Body.Close()
	_ = json.NewDecoder(r.Body).Decode(&h.body)
	w.WriteHeader(http.StatusNoContent)
}

func TestNotifierPostsOnFailedDeployment(t *testing.T) {
	var captured captureHandler
	srv := httptest.NewServer(&captured)
	defer srv.Close()

	n := New(srv.URL, &fakeAppSvc{}, nil)
	n.Handle(domain.EventDeploymentStatusChanged{
		DeploymentID:  42,
		ApplicationID: 5,
		Status:        domain.DeploymentFailed,
	})

	captured.mu.Lock()
	defer captured.mu.Unlock()
	if captured.requests != 1 {
		t.Fatalf("expected 1 request, got %d", captured.requests)
	}
	content, _ := captured.body["content"].(string)
	if content == "" {
		t.Fatal("expected content field")
	}
	if !strings.Contains(content, "❌") || !strings.Contains(content, "neo-portfolio") || !strings.Contains(content, "failed") {
		t.Fatalf("unexpected content: %q", content)
	}
}

func TestNotifierPostsOnSuccessDeployment(t *testing.T) {
	var captured captureHandler
	srv := httptest.NewServer(&captured)
	defer srv.Close()

	n := New(srv.URL, &fakeAppSvc{}, nil)
	n.Handle(domain.EventDeploymentStatusChanged{
		DeploymentID:  43,
		ApplicationID: 5,
		Status:        domain.DeploymentSuccess,
	})

	captured.mu.Lock()
	defer captured.mu.Unlock()
	if captured.requests != 1 {
		t.Fatalf("expected 1 request, got %d", captured.requests)
	}
	content, _ := captured.body["content"].(string)
	if !strings.Contains(content, "✅") || !strings.Contains(content, "succeeded") {
		t.Fatalf("unexpected content: %q", content)
	}
}

func TestNotifierSkipsIntermediateStatus(t *testing.T) {
	var captured captureHandler
	srv := httptest.NewServer(&captured)
	defer srv.Close()

	n := New(srv.URL, &fakeAppSvc{}, nil)
	n.Handle(domain.EventDeploymentStatusChanged{
		DeploymentID:  42,
		ApplicationID: 5,
		Status:        domain.DeploymentDeploying,
	})

	captured.mu.Lock()
	defer captured.mu.Unlock()
	if captured.requests != 0 {
		t.Fatalf("expected 0 requests for intermediate status, got %d", captured.requests)
	}
}

func TestNotifierNoopWhenURLUnset(t *testing.T) {
	var captured captureHandler
	srv := httptest.NewServer(&captured)
	defer srv.Close()

	n := New("", &fakeAppSvc{}, nil)
	n.Handle(domain.EventDeploymentStatusChanged{
		DeploymentID:  42,
		ApplicationID: 5,
		Status:        domain.DeploymentSuccess,
	})

	captured.mu.Lock()
	defer captured.mu.Unlock()
	if captured.requests != 0 {
		t.Fatalf("expected 0 requests when URL unset, got %d", captured.requests)
	}
}
