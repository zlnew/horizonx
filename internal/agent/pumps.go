package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"horizonx-server/internal/domain"

	"github.com/gorilla/websocket"
)

type MessageType struct {
	Type string `json:"type"`
}

func (a *Agent) readPump(ctx context.Context) error {
	a.conn.SetReadLimit(maxMessageSize)
	a.conn.SetReadDeadline(time.Now().Add(pongWait))
	a.conn.SetPongHandler(func(string) error {
		a.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			_, rawMessage, err := a.conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					a.log.Error("websocket read error (unexpected close)", "error", err)
					return err
				} else {
					a.log.Info("websocket read finished (normal closure or ping/pong timeout)")
					return nil
				}
			}

			var msgType MessageType
			if err := json.Unmarshal(rawMessage, &msgType); err != nil {
				a.log.Error("invalid json format received (cannot determine type)", "error", err)
				continue
			}

			if msgType.Type != "command" {
				a.log.Debug("ignoring non-command message from server", "type", msgType.Type)
				continue
			}

			var cmd ServerCommand
			if err := json.Unmarshal(rawMessage, &cmd); err != nil {
				a.log.Error("invalid command payload received", "error", err)
				continue
			}

			if cmd.Command == "init" {
				if err := a.handleInitCommand(cmd.Payload); err != nil {
					a.log.Error("failed to handle init command", "error", err)
				}
				continue
			}

			a.handleCommand(ctx, cmd)
		}
	}
}

func (a *Agent) handleInitCommand(payload json.RawMessage) error {
	var initPayload struct {
		ServerID string `json:"server_id"`
	}
	if err := json.Unmarshal(payload, &initPayload); err != nil {
		return fmt.Errorf("failed to unmarshal init payload: %w", err)
	}

	id, err := strconv.ParseInt(initPayload.ServerID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid server id received in init (not int64): %s, error: %w", initPayload.ServerID, err)
	}

	select {
	case a.initCh <- id:
		a.log.Debug("received server id via init command")
	default:
		a.log.Warn("init channel full, dropping init ID (should not happen)")
	}
	return nil
}

func (a *Agent) writePump(ctx context.Context) error {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case metric, ok := <-a.metricsCh:
			if !ok {
				a.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return nil
			}

			channel := domain.GetServerMetricsChannel(metric.ServerID)
			event := domain.EventMetricsReport
			msg := struct {
				Type    string `json:"type"`
				Channel string `json:"channel"`
				Event   string `json:"event"`
				Payload any    `json:"payload"`
			}{
				Type:    "event",
				Channel: channel,
				Event:   event,
				Payload: metric,
			}

			a.conn.SetWriteDeadline(time.Now().Add(writeWait))

			bytes, err := json.Marshal(msg)
			if err != nil {
				a.log.Error("failed to marshal metric", "error", err)
				continue
			}

			if err := a.conn.WriteMessage(websocket.TextMessage, bytes); err != nil {
				a.log.Error("failed to write metric", "error", err)
				return err
			}

		case <-ticker.C:
			a.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := a.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				a.log.Error("failed to write ping", "error", err)
				return err
			}
		}
	}
}
