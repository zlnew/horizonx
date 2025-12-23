package subscribers

import (
	"fmt"

	"horizonx-server/internal/adapters/ws/userws"
	"horizonx-server/internal/domain"
)

type DeploymentCommitInfoReceived struct {
	hub *userws.Hub
}

func NewDeploymentCommitInfoReceived(hub *userws.Hub) *DeploymentCommitInfoReceived {
	return &DeploymentCommitInfoReceived{hub: hub}
}

func (s *DeploymentCommitInfoReceived) Handle(event any) {
	evt, ok := event.(domain.EventDeploymentCommitInfoReceived)
	if !ok {
		return
	}

	s.hub.Broadcast(&domain.WsServerEvent{
		Channel: fmt.Sprintf("deployment:%d", evt.DeploymentID),
		Event:   "deployment_commit_info_received",
		Payload: evt,
	})

	s.hub.Broadcast(&domain.WsServerEvent{
		Channel: "deployments",
		Event:   "deployment_commit_info_received",
		Payload: evt,
	})
}
