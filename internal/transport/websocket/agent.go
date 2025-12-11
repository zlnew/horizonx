package websocket

import (
	"context"
	"encoding/json"
	"strconv"

	"horizonx-server/internal/domain"
)

func (h *Hub) initAgent(serverID string, client *Client) {
	payload := map[string]any{
		"type":    "command",
		"command": "init",
		"payload": map[string]string{
			"server_id": serverID,
		},
	}
	bytes, _ := json.Marshal(payload)

	select {
	case client.send <- bytes:
		h.log.Info("ws: agent registered, sent init command", "server_id", serverID)
	default:
		h.log.Info("ws: agent send buffer full during init", "server_id", serverID)
	}
}

func (h *Hub) updateAgentServerStatus(serverID string, isOnline bool) {
	parsedID, err := strconv.ParseInt(serverID, 10, 64)
	if err != nil {
		h.log.Error("ws: failed to parse server ID for status update", "id", serverID, "error", err)
		return
	}

	err = h.serverService.UpdateStatus(context.Background(), parsedID, isOnline)
	if err != nil {
		h.log.Error("ws: failed to update agent server status", "error", err, "server_id", parsedID, "online", isOnline)
	}

	h.Emit(domain.ChannelServerStatus, domain.EventServerStatusUpdated, domain.ServerStatusPayload{
		ServerID: parsedID,
		IsOnline: isOnline,
	})
}
