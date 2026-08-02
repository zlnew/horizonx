package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"horizonx/internal/domain"
	"horizonx/internal/logger"
	"horizonx/internal/version"

	"github.com/google/uuid"
)

type fakeServerService struct{}

func (f *fakeServerService) List(ctx context.Context, opts domain.ServerListOptions) (*domain.ListResult[*domain.Server], error) {
	return nil, nil
}
func (f *fakeServerService) GetByID(ctx context.Context, serverID uuid.UUID) (*domain.Server, error) {
	return nil, nil
}
func (f *fakeServerService) Register(ctx context.Context, req domain.ServerSaveRequest) (*domain.Server, string, error) {
	return nil, "", nil
}
func (f *fakeServerService) Update(ctx context.Context, req domain.ServerSaveRequest, serverID uuid.UUID) error {
	return nil
}
func (f *fakeServerService) UpdateOSInfo(ctx context.Context, serverID uuid.UUID, osInfo domain.OSInfo) error {
	return nil
}
func (f *fakeServerService) UpdateStatus(ctx context.Context, serverID uuid.UUID, status bool) error {
	return nil
}
func (f *fakeServerService) Delete(ctx context.Context, serverID uuid.UUID) error {
	return nil
}
func (f *fakeServerService) AuthorizeAgent(ctx context.Context, serverID uuid.UUID, secret string) (*domain.Server, error) {
	return &domain.Server{ID: serverID}, nil
}

type warnCaptureLogger struct {
	warnings []string
}

func (c *warnCaptureLogger) Debug(msg string, args ...any) {}
func (c *warnCaptureLogger) Info(msg string, args ...any)  {}
func (c *warnCaptureLogger) Warn(msg string, args ...any) {
	c.warnings = append(c.warnings, msg)
}
func (c *warnCaptureLogger) Error(msg string, args ...any) {}

func agentRequestWithVersion(hdr string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/agent/jobs", nil)
	req.Header.Set("Authorization", "Bearer "+uuid.NewString()+".test-secret")
	if hdr != "" {
		req.Header.Set(version.AgentVersionHeader, hdr)
	}
	return req
}

func TestAgentWarnsOnVersionMismatch(t *testing.T) {
	cl := &warnCaptureLogger{}
	handler := Agent(&fakeServerService{}, logger.Logger(cl))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Force a mismatch by simulating a different server version.
	orig := version.Version
	version.Version = "9.9.9"
	defer func() { version.Version = orig }()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, agentRequestWithVersion("1.0.0"))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if len(cl.warnings) != 1 || cl.warnings[0] != "agent version mismatch" {
		t.Fatalf("expected one version mismatch warning, got %v", cl.warnings)
	}
}

func TestAgentNoWarningOnMatchingVersion(t *testing.T) {
	cl := &warnCaptureLogger{}
	handler := Agent(&fakeServerService{}, logger.Logger(cl))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, agentRequestWithVersion(version.Version))

	if len(cl.warnings) != 0 {
		t.Fatalf("expected no warnings for matching version, got %v", cl.warnings)
	}
}

func TestAgentNoWarningWithoutHeader(t *testing.T) {
	cl := &warnCaptureLogger{}
	handler := Agent(&fakeServerService{}, logger.Logger(cl))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, agentRequestWithVersion(""))

	if len(cl.warnings) != 0 {
		t.Fatalf("expected no warnings without header, got %v", cl.warnings)
	}
}
