package subscribers

import (
	"fmt"

	"horizonx-server/internal/adapters/ws/userws"
	"horizonx-server/internal/domain"
)

type DeploymentStatusChanged struct {
	hub *userws.Hub
}

func NewDeploymentStatusChanged(hub *userws.Hub) *DeploymentStatusChanged {
	return &DeploymentStatusChanged{hub: hub}
}

func (s *DeploymentStatusChanged) Handle(event any) {
	evt, ok := event.(domain.EventDeploymentStatusChanged)
	if !ok {
		return
	}

	s.hub.Broadcast(&domain.WsServerEvent{
		Channel: fmt.Sprintf("deployment:%d", evt.DeploymentID),
		Event:   "deployment_status_changed",
		Payload: evt,
	})

	s.hub.Broadcast(&domain.WsServerEvent{
		Channel: fmt.Sprintf("application:%d", evt.ApplicationID),
		Event:   "deployment_status_changed",
		Payload: evt,
	})
}
