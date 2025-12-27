package subscribers

import (
	"fmt"

	"horizonx-server/internal/adapters/ws/userws"
	"horizonx-server/internal/domain"
)

type DeploymentStarted struct {
	hub *userws.Hub
}

func NewDeploymentStarted(hub *userws.Hub) *DeploymentStarted {
	return &DeploymentStarted{hub: hub}
}

func (s *DeploymentStarted) Handle(event any) {
	evt, ok := event.(domain.EventDeploymentStarted)
	if !ok {
		return
	}

	s.hub.Broadcast(&domain.WsServerEvent{
		Channel: fmt.Sprintf("deployment:%d", evt.DeploymentID),
		Event:   "deployment_started",
		Payload: evt,
	})
}
