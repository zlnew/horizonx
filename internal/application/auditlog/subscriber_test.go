package auditlog

import (
	"context"
	"encoding/json"
	"testing"

	"horizonx/internal/domain"

	"github.com/google/uuid"
)

type fakeAuditRepo struct {
	logs []*domain.AuditLog
}

func (f *fakeAuditRepo) Create(ctx context.Context, log *domain.AuditLog) (*domain.AuditLog, error) {
	log.ID = int64(len(f.logs) + 1)
	f.logs = append(f.logs, log)
	return log, nil
}

func (f *fakeAuditRepo) List(ctx context.Context, opts domain.AuditLogListOptions) ([]*domain.AuditLog, int64, error) {
	return f.logs, int64(len(f.logs)), nil
}

func TestSubscriberRecordsDeploymentCreated(t *testing.T) {
	svc := NewService(&fakeAuditRepo{})
	sub := NewSubscriber(svc)

	sub.OnDeploymentCreated(domain.EventDeploymentCreated{DeploymentID: 7, ApplicationID: 3, DeployedBy: 42})

	res, _ := svc.List(context.Background(), domain.AuditLogListOptions{})
	if len(res.Data) != 1 {
		t.Fatalf("expected 1 log, got %d", len(res.Data))
	}
	l := res.Data[0]
	if l.Action != "deployment.created" || l.ResourceType != "deployment" || l.ResourceID != "7" {
		t.Fatalf("unexpected log: %+v", l)
	}
	if l.ActorID == nil || *l.ActorID != 42 {
		t.Fatalf("expected actor 42, got %v", l.ActorID)
	}
}

func TestSubscriberRecordsStatusAndServer(t *testing.T) {
	svc := NewService(&fakeAuditRepo{})
	sub := NewSubscriber(svc)

	sub.OnDeploymentStatusChanged(domain.EventDeploymentStatusChanged{DeploymentID: 8, ApplicationID: 3, Status: domain.DeploymentSuccess})
	sub.OnServerStatusChanged(domain.EventServerStatusChanged{ServerID: uuid.New(), IsOnline: true})

	res, _ := svc.List(context.Background(), domain.AuditLogListOptions{})
	if len(res.Data) != 2 {
		t.Fatalf("expected 2 logs, got %d", len(res.Data))
	}
	if res.Data[0].Action != "deployment.success" {
		t.Fatalf("unexpected action: %s", res.Data[0].Action)
	}
	if res.Data[1].Action != "server.status.online" {
		t.Fatalf("unexpected action: %s", res.Data[1].Action)
	}
}

func TestSubscriberIgnoresWrongPayload(t *testing.T) {
	svc := NewService(&fakeAuditRepo{})
	sub := NewSubscriber(svc)

	sub.OnDeploymentCreated("not a deployment event") // wrong type → ignored

	res, _ := svc.List(context.Background(), domain.AuditLogListOptions{})
	if res.Meta.Total != 0 || len(res.Data) != 0 {
		t.Fatalf("expected no logs, got %d", res.Meta.Total)
	}
}

func TestServiceCreateMarshalsDetails(t *testing.T) {
	repo := &fakeAuditRepo{}
	svc := NewService(repo)

	_, err := svc.Create(context.Background(), nil, "test.action", "test", "1", map[string]any{"k": "v"})
	if err != nil {
		t.Fatal(err)
	}
	got := string(repo.logs[0].Details)
	if !json.Valid(repo.logs[0].Details) || got != `{"k":"v"}` {
		t.Fatalf("unexpected details: %s", got)
	}
}
