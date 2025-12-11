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
		h.log.Info("sent init command to agent", "server_id", serverID)
	default:
		h.log.Info("agent send buffer full during init", "server_id", serverID)
	}
}

func (h *Hub) updateAgentStatus(serverID string, isOnline bool) {
	go func(serverID string, isOnline bool) {
		parsedServerID, err := strconv.ParseInt(serverID, 10, 64)
		if err != nil {
			h.log.Error("failed to parse server id to int64 for status update", "error", err, "server_id_string", serverID, "online", isOnline)
			return
		}

		err = h.serverService.UpdateStatus(context.Background(), parsedServerID, isOnline)
		if err != nil {
			h.log.Error("failed to update agent server status", "error", err, "server_id", parsedServerID, "online", isOnline)
		} else {
			h.Emit(domain.ChannelServer, domain.EventServerStatusUpdated, domain.ServerStatusPayload{
				ServerID: parsedServerID,
				IsOnline: isOnline,
			})
			h.log.Debug("Agent DB status updated successfully", "server_id", parsedServerID, "online", isOnline)
		}
	}(serverID, isOnline)
}
