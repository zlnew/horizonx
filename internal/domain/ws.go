package domain

import (
	"encoding/json"

	"github.com/google/uuid"
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

type ServerStatusChanged struct {
	ServerID uuid.UUID `json:"server_id"`
	IsOnline bool      `json:"is_online"`
}
