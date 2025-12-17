// Package ws
package ws

import (
	"context"
	"time"

	"horizonx-server/internal/domain"
	"horizonx-server/internal/logger"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Agent struct {
	ctx    context.Context
	cancel context.CancelFunc

	hub  *AgentRouter
	conn *websocket.Conn
	send chan []byte

	log logger.Logger
	svc domain.ServerService

	ID uuid.UUID
}

func NewAgent(hub *AgentRouter, conn *websocket.Conn, log logger.Logger, svc domain.ServerService, cID uuid.UUID) *Agent {
	ctx, cancel := context.WithCancel(hub.ctx)

	return &Agent{
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

func (a *Agent) readPump() {
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
			_, _, err := a.conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					a.log.Warn("ws: agent disconnected unexpected", "error", err)
				}
				return
			}
		}
	}
}

func (a *Agent) writePump() {
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
