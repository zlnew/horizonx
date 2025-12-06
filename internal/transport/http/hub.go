// Package http
package http

import (
	"encoding/json"
	"net/http"
	"time"

	"zlnew/monitor-agent/internal/core"
	"zlnew/monitor-agent/internal/infra/logger"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
	log  logger.Logger
}

type Hub struct {
	rooms      map[string]map[*Client]bool
	broadcast  chan *Broadcast
	register   chan *Subscription
	unregister chan *Subscription
	log        logger.Logger
}

type Subscription struct {
	client *Client
	room   string
}

type Broadcast struct {
	room    string
	message []byte
}

type ClientMessage struct {
	Type    string `json:"type"`
	Channel string `json:"channel"`
}

func NewHub(log logger.Logger) *Hub {
	return &Hub{
		broadcast:  make(chan *Broadcast),
		register:   make(chan *Subscription),
		unregister: make(chan *Subscription),
		rooms:      make(map[string]map[*Client]bool),
		log:        log,
	}
}

func (h *Hub) Run() {
	for {
		select {
		case sub := <-h.register:
			if _, ok := h.rooms[sub.room]; !ok {
				h.rooms[sub.room] = make(map[*Client]bool)
			}
			h.rooms[sub.room][sub.client] = true
			h.log.Info("client subscribed", "room", sub.room, "remote_addr", sub.client.conn.RemoteAddr())
		case sub := <-h.unregister:
			if clients, ok := h.rooms[sub.room]; ok {
				if _, ok := clients[sub.client]; ok {
					delete(clients, sub.client)
					if len(clients) == 0 {

						delete(h.rooms, sub.room)
						h.log.Info("room closed", "room", sub.room)
					}
					h.log.Info("client unsubscribed", "room", sub.room, "remote_addr", sub.client.conn.RemoteAddr())
				}
			}
		case broadcast := <-h.broadcast:
			if clients, ok := h.rooms[broadcast.room]; ok {
				for client := range clients {
					select {
					case client.send <- broadcast.message:
					default:

						h.log.Warn("client send channel full, skipping", "remote_addr", client.conn.RemoteAddr())
					}
				}
			}
		}
	}
}

func (h *Hub) BroadcastMetrics(metrics core.Metrics) {
	msg := map[string]interface{}{
		"channel": "metrics",
		"payload": metrics,
	}
	bytes, err := json.Marshal(msg)
	if err != nil {
		h.log.Error("marshal metrics", "error", err)
		return
	}
	h.broadcast <- &Broadcast{room: "metrics", message: bytes}
}

func (c *Client) readPump() {
	subscribedRooms := make(map[string]bool)
	defer func() {
		for room := range subscribedRooms {
			c.hub.unregister <- &Subscription{client: c, room: room}
		}
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		var msg ClientMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			c.log.Error("unmarshal message", "error", err)
			continue
		}
		if msg.Type == "subscribe" {
			if !subscribedRooms[msg.Channel] {
				subscribedRooms[msg.Channel] = true
				c.hub.register <- &Subscription{client: c, room: msg.Channel}
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
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {

				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
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

func ServeWs(hub *Hub, w http.ResponseWriter, r *http.Request, log logger.Logger) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error("upgrade", "error", err)
		return
	}
	client := &Client{hub: hub, conn: conn, send: make(chan []byte, 256), log: log}

	go client.writePump()
	go client.readPump()

	log.Info("client connected", "remote_addr", conn.RemoteAddr())
}
