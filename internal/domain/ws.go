package domain

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

const (
	WsClientUser  = "USER"
	WsClientAgent = "AGENT"
)

const (
	WsChannelServerStatus          = "server_status"
	WsChannelServerMetricsTemplate = "server:%d:metrics"
)

const (
	WsEventAgentReady            = "agent_ready"
	WsEventServerStatusUpdated   = "server_status_updated"
	WsEventServerMetricsReport   = "server_metrics_report"
	WsEventServerMetricsReceived = "server_metrics_received"
)

const (
	WsCommandAgentInit = "agent_init"
)

const (
	WsAgentReport = "agent_report"
	WsSubscribe   = "subscribe"
	WsUnsubscribe = "unsubscribe"
)

type WsClientMessage struct {
	Type    string          `json:"type"`
	Channel string          `json:"channel,omitempty"`
	Event   string          `json:"event,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type WsServerEvent struct {
	Channel string `json:"channel"`
	Event   string `json:"event"`
	Payload any    `json:"payload,omitempty"`
}

type WsAgentCommand struct {
	TargetServerID uuid.UUID         `json:"target_server_id"`
	CommandType    string            `json:"command_type"`
	Payload        JobCommandPayload `json:"payload"`
}

type ServerStatusPayload struct {
	ServerID uuid.UUID `json:"server_id"`
	IsOnline bool      `json:"is_online"`
}

func GetServerMetricsChannel(serverID int64) string {
	return fmt.Sprintf(WsChannelServerMetricsTemplate, serverID)
}
