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
)

type stubLogger struct{}

func (stubLogger) Debug(msg string, args ...any) {}
func (stubLogger) Info(msg string, args ...any)  {}
func (stubLogger) Warn(msg string, args ...any)  {}
func (stubLogger) Error(msg string, args ...any) {}

type fakeAuditLogService struct {
	result *domain.ListResult[*domain.AuditLog]
	err    error
}

func (f *fakeAuditLogService) Create(ctx context.Context, actorID *int64, action, resourceType, resourceID string, details any) (*domain.AuditLog, error) {
	return nil, nil
}

func (f *fakeAuditLogService) List(ctx context.Context, opts domain.AuditLogListOptions) (*domain.ListResult[*domain.AuditLog], error) {
	return f.result, f.err
}

func newAuditLogTestHandler(svc domain.AuditLogService) *AuditLogHandler {
	log := stubLogger{}
	return NewAuditLogHandler(
		svc,
		request.NewJSONDecoder(),
		response.NewJSONWriter(log),
		nil,
	)
}

func TestAuditLogIndexReturnsFlatDataShape(t *testing.T) {
	now := time.Now().UTC()
	svc := &fakeAuditLogService{
		result: &domain.ListResult[*domain.AuditLog]{
			Data: []*domain.AuditLog{
				{
					ID:           1,
					Action:       "deployment.created",
					ResourceType: "deployment",
					ResourceID:   "42",
					CreatedAt:    now,
				},
			},
			Meta: domain.CalculateMeta(1, 1, 20),
		},
	}

	h := newAuditLogTestHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/audit-logs?page=1&limit=20&paginate=true", nil)
	rec := httptest.NewRecorder()

	h.Index(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body struct {
		Data []struct {
			ID int64 `json:"id"`
		} `json:"data"`
		Meta struct {
			Total int64 `json:"total"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(body.Data) != 1 {
		t.Fatalf("expected 1 data row at top level, got %d (nested shape regression?)", len(body.Data))
	}
	if body.Data[0].ID != 1 {
		t.Fatalf("expected data[0].id == 1, got %d", body.Data[0].ID)
	}
	if body.Meta.Total != 1 {
		t.Fatalf("expected meta.total == 1, got %d", body.Meta.Total)
	}
}

func TestAuditLogIndexEmptyList(t *testing.T) {
	svc := &fakeAuditLogService{
		result: &domain.ListResult[*domain.AuditLog]{
			Data: []*domain.AuditLog{},
			Meta: domain.CalculateMeta(0, 1, 20),
		},
	}

	h := newAuditLogTestHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/audit-logs", nil)
	rec := httptest.NewRecorder()

	h.Index(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body struct {
		Data []any `json:"data"`
		Meta any   `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if body.Data == nil {
		t.Fatal("expected data key present and an array, got null")
	}
	if body.Meta == nil {
		t.Fatal("expected meta key present, got null")
	}
}

func TestAuditLogIndexServiceError(t *testing.T) {
	svc := &fakeAuditLogService{err: context.DeadlineExceeded}

	h := newAuditLogTestHandler(svc)
	req := httptest.NewRequest(http.MethodGet, "/audit-logs", nil)
	rec := httptest.NewRecorder()

	h.Index(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
