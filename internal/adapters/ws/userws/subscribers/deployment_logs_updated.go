package subscribers

import (
	"fmt"

	"horizonx-server/internal/adapters/ws/userws"
	"horizonx-server/internal/domain"
)

type DeploymentLogsUpdated struct {
	hub *userws.Hub
}

func NewDeploymentLogsUpdated(hub *userws.Hub) *DeploymentLogsUpdated {
	return &DeploymentLogsUpdated{hub: hub}
}

func (s *DeploymentLogsUpdated) Handle(event any) {
	evt, ok := event.(domain.EventDeploymentLogsUpdated)
	if !ok {
		return
	}

	// Broadcast to deployment-specific channel
	s.hub.Broadcast(&domain.WsServerEvent{
		Channel: fmt.Sprintf("deployment:%d", evt.DeploymentID),
		Event:   "deployment_logs_updated",
		Payload: evt,
	})

	// Also broadcast to application channel
	s.hub.Broadcast(&domain.WsServerEvent{
		Channel: fmt.Sprintf("application:%d", evt.ApplicationID),
		Event:   "deployment_logs_updated",
		Payload: evt,
	})
}
