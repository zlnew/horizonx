package subscribers

import (
	"horizonx-server/internal/adapters/ws/userws"
	"horizonx-server/internal/domain"
)

type LogReceived struct {
	hub *userws.Hub
}

func NewLogReceived(hub *userws.Hub) *LogReceived {
	return &LogReceived{hub: hub}
}

func (s *LogReceived) Handle(event any) {
	evt, ok := event.(domain.Log)
	if !ok {
		return
	}

	s.hub.Broadcast(&domain.WsServerEvent{
		Channel: "logs",
		Event:   "log_received",
		Payload: evt,
	})
}
