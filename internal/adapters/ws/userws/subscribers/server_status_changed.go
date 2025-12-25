package subscribers

import (
	"fmt"

	"horizonx-server/internal/adapters/ws/userws"
	"horizonx-server/internal/domain"
)

type ServerStatusChanged struct {
	hub *userws.Hub
}

func NewServerStatusChanged(hub *userws.Hub) *ServerStatusChanged {
	return &ServerStatusChanged{hub: hub}
}

func (s *ServerStatusChanged) Handle(event any) {
	evt, ok := event.(domain.EventServerStatusChanged)
	if !ok {
		return
	}

	s.hub.Broadcast(&domain.WsServerEvent{
		Channel: fmt.Sprintf("server:%s", evt.ServerID.String()),
		Event:   "server_status_changed",
		Payload: event,
	})

	s.hub.Broadcast(&domain.WsServerEvent{
		Channel: "servers",
		Event:   "server_status_changed",
		Payload: event,
	})
}
