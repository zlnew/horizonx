package job

import (
	"time"

	"horizonx-server/internal/domain"
	"horizonx-server/internal/logger"
	"horizonx-server/internal/transport/ws"
)

type JobDispatcher struct {
	hub *ws.AgentHub
	log logger.Logger
}

func NewJobDispatcher(hub *ws.AgentHub, log logger.Logger) *JobDispatcher {
	return &JobDispatcher{
		hub: hub,
		log: log,
	}
}

func (d *JobDispatcher) OnJobCreated(e any) {
	ev := e.(domain.EventJobCreated)

	switch ev.JobType {
	case "agent_init":
		retryConfig := &domain.JobRetryConfig{
			MaxAttempts: 5,
			BaseDelay:   100 * time.Millisecond,
		}

		command := &domain.WsAgentCommand{
			TargetServerID: ev.ServerID,
			CommandType:    domain.WsCommandAgentInit,
			Payload:        domain.JobCommandPayload{JobID: ev.JobID},
		}

		d.hub.SendCommand(command, *retryConfig)

	default:
	}
}
