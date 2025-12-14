package ws

import (
	"horizonx-server/internal/domain"
)

type ServerStatusSubscriber struct {
	hub *Hub
}

func NewServerStatusSubscriber(hub *Hub) *ServerStatusSubscriber {
	return &ServerStatusSubscriber{hub: hub}
}

func (s *ServerStatusSubscriber) Handle(event domain.ServerStatusChanged) {
	s.hub.Broadcast(&domain.WsServerEvent{
		Channel: "server_status",
		Event:   "server_status_changed",
		Payload: event,
	})
}
