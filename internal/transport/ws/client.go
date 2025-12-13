// Package ws
package ws

import (
	"context"
	"encoding/json"
	"time"

	"horizonx-server/internal/domain"
	"horizonx-server/internal/logger"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 8192
)

type Client struct {
	ctx    context.Context
	cancel context.CancelFunc

	hub  *Hub
	conn *websocket.Conn
	send chan []byte

	log logger.Logger

	ID   string
	Type string
}

func NewClient(hub *Hub, conn *websocket.Conn, log logger.Logger, cID string, cType string) *Client {
	ctx, cancel := context.WithCancel(hub.ctx)

	return &Client{
		ctx:    ctx,
		cancel: cancel,

		hub:  hub,
		conn: conn,
		send: make(chan []byte, 256),

		log: log,

		ID:   cID,
		Type: cType,
	}
}

func (c *Client) readPump() {
	defer func() {
		c.cancel()
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			_, message, err := c.conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					c.log.Warn("ws: client disconnected unexpected", "error", err)
				}
				return
			}

			var msg domain.WsClientMessage
			if err := json.Unmarshal(message, &msg); err != nil {
				c.log.Error("ws: invalid client message", "error", err)
				continue
			}

			switch msg.Type {
			case "subscribe":
				c.hub.subscribe <- &Subscription{
					client:  c,
					channel: msg.Channel,
				}
			case "unsubscribe":
				c.hub.unsubscribe <- &Subscription{
					client:  c,
					channel: msg.Channel,
				}
			default:
				c.log.Debug("ws: unknown client message type", "type", msg.Type)
			}
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case <-c.ctx.Done():
			return

		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
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
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
