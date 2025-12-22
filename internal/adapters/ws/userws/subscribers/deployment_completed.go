package subscribers

import (
	"fmt"

	"horizonx-server/internal/adapters/ws/userws"
	"horizonx-server/internal/domain"
)

type DeploymentCompleted struct {
	hub *userws.Hub
}

func NewDeploymentCompleted(hub *userws.Hub) *DeploymentCompleted {
	return &DeploymentCompleted{hub: hub}
}

func (s *DeploymentCompleted) Handle(event any) {
	evt, ok := event.(domain.EventDeploymentCompleted)
	if !ok {
		return
	}

	s.hub.Broadcast(&domain.WsServerEvent{
		Channel: fmt.Sprintf("deployment:%d", evt.DeploymentID),
		Event:   "deployment_completed",
		Payload: evt,
	})

	s.hub.Broadcast(&domain.WsServerEvent{
		Channel: fmt.Sprintf("application:%d", evt.ApplicationID),
		Event:   "deployment_completed",
		Payload: evt,
	})
}
