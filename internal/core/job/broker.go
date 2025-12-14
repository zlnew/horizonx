package job

import (
	"context"

	"horizonx-server/internal/domain"
	"horizonx-server/internal/logger"
	"horizonx-server/internal/transport/ws"
)

type JobBrokerDeps struct {
	Server domain.ServerService
}

type JobBroker struct {
	hub  *ws.Hub
	log  logger.Logger
	deps *JobBrokerDeps
}

func NewJobBroker(hub *ws.Hub, log logger.Logger, deps *JobBrokerDeps) *JobBroker {
	return &JobBroker{
		hub:  hub,
		log:  log,
		deps: deps,
	}
}

func (h *JobBroker) OnJobFinished(e any) {
	ev := e.(domain.EventJobFinished)

	switch ev.JobType {
	case "agent_init":
		if err := h.deps.Server.UpdateStatus(context.Background(), ev.ServerID, true); err != nil {
			h.log.Error("failed to update server status")
			return
		}

		h.hub.Broadcast(&domain.WsServerEvent{
			Channel: "server_status",
			Event:   domain.WsEventServerStatusUpdated,
			Payload: &domain.ServerStatusPayload{
				ServerID: ev.ServerID,
				IsOnline: true,
			},
		})

	default:
	}
}
