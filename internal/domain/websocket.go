package domain

import "fmt"

const (
	ChannelServer                = "servers"
	ChannelServerMetricsTemplate = "server:%d:metrics"
)

const (
	EventServerStatusUpdated = "server_status_updated"
	EventMetricsReport       = "metrics_report"
	EventMetricsReceived     = "metrics_received"
)

type ServerStatusPayload struct {
	ServerID int64
	IsOnline bool
}

func GetServerMetricsChannel(serverID int64) string {
	return fmt.Sprintf(ChannelServerMetricsTemplate, serverID)
}
