package subscribers

import (
	"fmt"

	"horizonx/internal/adapters/ws/userws"
	"horizonx/internal/domain"
)

// ContainerLogChunk relays agent container-log chunks to the WS channel
// `app_logs:{applicationID}` — the channel the application logs page is
// subscribed to. Live tail and historical query share this one path; the
// only difference is whether the chunk carries EOF:true.
type ContainerLogChunk struct {
	hub *userws.Hub
}

func NewContainerLogChunk(hub *userws.Hub) *ContainerLogChunk {
	return &ContainerLogChunk{hub: hub}
}

func (s *ContainerLogChunk) Handle(event any) {
	evt, ok := event.(*domain.ContainerLogChunk)
	if !ok {
		return
	}

	s.hub.Broadcast(&domain.WsServerEvent{
		Channel: fmt.Sprintf("app_logs:%d", evt.ApplicationID),
		Event:   "container_log_chunk",
		Payload: evt,
	})
}
