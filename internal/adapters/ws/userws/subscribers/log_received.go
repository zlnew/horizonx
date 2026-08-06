package subscribers

import (
	"fmt"

	"horizonx/internal/adapters/ws/userws"
	"horizonx/internal/domain"
)

type LogReceived struct {
	hub *userws.Hub
}

func NewLogReceived(hub *userws.Hub) *LogReceived {
	return &LogReceived{hub: hub}
}

// logChannels returns the WS channels a log event should be broadcast to.
// Scoping keeps a verbose deploy on one app from flooding every dashboard:
//   - job:<id>         -> activity/job detail page
//   - deployment:<id>  -> deploy detail page
//   - logs             -> fallback (server-level logs without context)
// A deploy log usually carries BOTH a job and a deployment ID, so both
// channels are returned and each page sees its logs.
func logChannels(evt *domain.Log) []string {
	channels := make([]string, 0, 2)
	if evt.JobID != nil {
		channels = append(channels, fmt.Sprintf("job:%d", *evt.JobID))
	}
	if evt.DeploymentID != nil {
		channels = append(channels, fmt.Sprintf("deployment:%d", *evt.DeploymentID))
	}
	if len(channels) == 0 {
		channels = append(channels, "logs")
	}
	return channels
}

func (s *LogReceived) Handle(event any) {
	evt, ok := event.(*domain.Log)
	if !ok {
		return
	}

	for _, channel := range logChannels(evt) {
		s.hub.Broadcast(&domain.WsServerEvent{
			Channel: channel,
			Event:   "log_received",
			Payload: evt,
		})
	}
}
