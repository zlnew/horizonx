// Package agentws
package agentws

import (
	"context"
	"encoding/json"
	"time"

	"horizonx/internal/domain"
	"horizonx/internal/logger"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	// agentws carries server→agent commands (log batches in A2 can reach
	// hundreds of KB) — 256KB cap. userws stays at the default 8KB.
	maxMessageSize = 262144
)

type Client struct {
	ctx    context.Context
	cancel context.CancelFunc

	hub  *Router
	conn *websocket.Conn
	send chan []byte

	log logger.Logger
	svc  domain.ServerService

	// publish forwards events onto the server event bus (wired by the
	// handler; nil is a safe no-op).
	publish func(eventName string, event any)

	ID uuid.UUID
}

func NewClient(hub *Router, conn *websocket.Conn, log logger.Logger, svc domain.ServerService, cID uuid.UUID) *Client {
	ctx, cancel := context.WithCancel(hub.ctx)

	return &Client{
		ctx:    ctx,
		cancel: cancel,

		hub:  hub,
		conn: conn,
		send: make(chan []byte, 256),

		log: log,
		svc: svc,

		ID: cID,
	}
}

func (a *Client) readPump() {
	defer func() {
		a.cancel()
		a.hub.unregister <- a

		if err := a.svc.UpdateStatus(context.Background(), a.ID, false); err != nil {
			a.log.Error("ws: failed to set server offline", "server_id", a.ID.String())
		}

		a.conn.Close()
	}()

	a.conn.SetReadLimit(maxMessageSize)
	a.conn.SetReadDeadline(time.Now().Add(pongWait))
	a.conn.SetPongHandler(func(string) error {
		a.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		select {
		case <-a.ctx.Done():
			return

		default:
			_, message, err := a.conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					a.log.Warn("ws: agent disconnected unexpected", "error", err)
				}
				return
			}

			var msg domain.WsAgentMessage
			if err := json.Unmarshal(message, &msg); err != nil {
				a.log.Error("ws: invalid agent message", "error", err)
				continue
			}

			srvID := a.ID.String()

			switch msg.Event {
			case "server_os_info":
				var osInfo domain.OSInfo
				if err := json.Unmarshal(msg.Payload, &osInfo); err != nil {
					a.log.Error("ws: failed to unmarshal OS info payload", "error", err)
					break
				}

				if err := a.svc.UpdateOSInfo(context.Background(), a.ID, osInfo); err != nil {
					a.log.Error("ws: failed to update server os info", "error", err)
					break
				}

			case "container_log_chunk":
				var chunk domain.ContainerLogChunk
				if err := json.Unmarshal(msg.Payload, &chunk); err != nil {
					a.log.Error("ws: failed to unmarshal container log chunk", "error", err)
					break
				}
				if a.publish != nil {
					a.publish("container_log_chunk", &chunk)
				}
				a.log.Debug("ws: container log chunk relayed", "server_id", srvID, "stream_id", chunk.StreamID, "seq", chunk.Seq, "lines", len(chunk.Lines), "eof", chunk.EOF)

			default:
				a.log.Debug("ws: unknown agent message event", "event", msg.Event)
			}
		}
	}
}

func (a *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		a.conn.Close()
	}()

	for {
		select {
		case <-a.ctx.Done():
			return

		case message, ok := <-a.send:
			a.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				a.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := a.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}

			_, err = w.Write(message)
			if err != nil {
				w.Close()
				return
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			a.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := a.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
