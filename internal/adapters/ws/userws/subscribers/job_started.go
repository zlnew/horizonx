package subscribers

import (
	"fmt"

	"horizonx-server/internal/adapters/ws/userws"
	"horizonx-server/internal/domain"
)

type JobStarted struct {
	hub *userws.Hub
}

func NewJobStarted(hub *userws.Hub) *JobStarted {
	return &JobStarted{hub: hub}
}

func (s *JobStarted) Handle(event any) {
	evt, ok := event.(domain.EventJobStarted)
	if !ok {
		return
	}

	s.hub.Broadcast(&domain.WsServerEvent{
		Channel: fmt.Sprintf("job:%d", evt.JobID),
		Event:   "job_started",
		Payload: evt,
	})
}
