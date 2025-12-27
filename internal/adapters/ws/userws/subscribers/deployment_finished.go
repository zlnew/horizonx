package subscribers

import (
	"fmt"

	"horizonx-server/internal/adapters/ws/userws"
	"horizonx-server/internal/domain"
)

type DeploymentFinished struct {
	hub *userws.Hub
}

func NewDeploymentFinished(hub *userws.Hub) *DeploymentFinished {
	return &DeploymentFinished{hub: hub}
}

func (s *DeploymentFinished) Handle(event any) {
	evt, ok := event.(domain.EventDeploymentFinished)
	if !ok {
		return
	}

	s.hub.Broadcast(&domain.WsServerEvent{
		Channel: fmt.Sprintf("deployment:%d", evt.DeploymentID),
		Event:   "deployment_finished",
		Payload: evt,
	})
}
