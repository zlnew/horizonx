// Package auditlog — append-only record of user + system actions.
package auditlog

import (
	"context"
	"strconv"

	"horizonx/internal/domain"
	"horizonx/internal/event"
)

// Subscriber records audit entries from domain events. It subscribes to the
// event bus (like the webhook notifier) so no handler needs wiring changes.
type Subscriber struct {
	svc domain.AuditLogService
}

func NewSubscriber(svc domain.AuditLogService) *Subscriber {
	return &Subscriber{svc: svc}
}

// Register subscribes to the domain events worth auditing.
func (s *Subscriber) Register(bus *event.Bus) {
	bus.Subscribe("deployment_created", s.OnDeploymentCreated)
	bus.Subscribe("deployment_status_changed", s.OnDeploymentStatusChanged)
	bus.Subscribe("application_created", s.OnApplicationCreated)
	bus.Subscribe("server_status_changed", s.OnServerStatusChanged)
}

func (s *Subscriber) OnDeploymentCreated(event any) {
	e, ok := event.(domain.EventDeploymentCreated)
	if !ok {
		return
	}
	actor := e.DeployedBy
	_, _ = s.svc.Create(context.Background(), &actor, "deployment.created", "deployment", strconv.FormatInt(e.DeploymentID, 10), map[string]any{
		"application_id": e.ApplicationID,
	})
}

func (s *Subscriber) OnDeploymentStatusChanged(event any) {
	e, ok := event.(domain.EventDeploymentStatusChanged)
	if !ok {
		return
	}
	_, _ = s.svc.Create(context.Background(), nil, "deployment."+string(e.Status), "deployment", strconv.FormatInt(e.DeploymentID, 10), map[string]any{
		"application_id": e.ApplicationID,
		"status":         e.Status,
	})
}

func (s *Subscriber) OnApplicationCreated(event any) {
	e, ok := event.(domain.EventApplicationCreated)
	if !ok {
		return
	}
	_, _ = s.svc.Create(context.Background(), nil, "application.created", "application", strconv.FormatInt(e.ApplicationID, 10), map[string]any{
		"server_id": e.ServerID.String(),
	})
}

func (s *Subscriber) OnServerStatusChanged(event any) {
	e, ok := event.(domain.EventServerStatusChanged)
	if !ok {
		return
	}
	state := "offline"
	if e.IsOnline {
		state = "online"
	}
	_, _ = s.svc.Create(context.Background(), nil, "server.status."+state, "server", e.ServerID.String(), nil)
}
