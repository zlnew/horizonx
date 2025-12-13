package ws

import (
	"context"
	"encoding/json"

	"horizonx-server/internal/domain"
	"horizonx-server/internal/logger"
)

type Hub struct {
	ctx    context.Context
	cancel context.CancelFunc

	clients  map[*Client]bool
	channels map[string]map[*Client]bool

	register    chan *Client
	unregister  chan *Client
	subscribe   chan *Subscription
	unsubscribe chan *Subscription
	events      chan *domain.WsServerEvent

	log logger.Logger
}

type Subscription struct {
	client  *Client
	channel string
}

func NewHub(parent context.Context, log logger.Logger) *Hub {
	ctx, cancel := context.WithCancel(parent)

	return &Hub{
		ctx:    ctx,
		cancel: cancel,

		clients:  make(map[*Client]bool),
		channels: make(map[string]map[*Client]bool),

		register:    make(chan *Client, 64),
		unregister:  make(chan *Client, 64),
		subscribe:   make(chan *Subscription, 64),
		unsubscribe: make(chan *Subscription, 64),
		events:      make(chan *domain.WsServerEvent, 256),

		log: log,
	}
}

func (h *Hub) Run() {
	for {
		select {
		case <-h.ctx.Done():
			h.log.Info("ws: hub shutting down...")
			for client := range h.clients {
				close(client.send)
			}
			return

		case c := <-h.register:
			h.clients[c] = true
			c.log.Info("ws: client registered", "id", c.ID, "type", c.Type)

		case c := <-h.unregister:
			if !h.clients[c] {
				continue
			}

			delete(h.clients, c)
			close(c.send)
			h.log.Info("ws: client unregistered", "id", c.ID, "type", c.Type)

			for chID, subs := range h.channels {
				if _, subsribed := subs[c]; subsribed {
					delete(subs, c)
					if len(subs) == 0 {
						delete(h.channels, chID)
					}
				}
			}

		case sub := <-h.subscribe:
			if h.channels[sub.channel] == nil {
				h.channels[sub.channel] = make(map[*Client]bool)
			}
			h.channels[sub.channel][sub.client] = true

		case sub := <-h.unsubscribe:
			if subs, ok := h.channels[sub.channel]; ok {
				if _, subscribed := subs[sub.client]; subscribed {
					delete(subs, sub.client)
					if len(subs) == 0 {
						delete(h.channels, sub.channel)
					}
				}
			}

		case ev := <-h.events:
			h.handleEvent(ev)
		}
	}
}

func (h *Hub) Stop() {
	h.cancel()
}

func (h *Hub) Broadcast(ev *domain.WsServerEvent) {
	select {
	case h.events <- ev:
	case <-h.ctx.Done():
	default:
		h.log.Warn("ws: broadcast buffer full, dropping event")
	}
}

func (h *Hub) handleEvent(ev *domain.WsServerEvent) {
	message, err := json.Marshal(ev)
	if err != nil {
		h.log.Error("ws: failed to marshal server event", "error", err)
		return
	}

	targetClients := h.clients

	if ev.Channel != "" {
		if subs, ok := h.channels[ev.Channel]; ok {
			targetClients = subs
		} else {
			h.log.Debug("ws: event channels has no subscribers", "channel", ev.Channel)
			return
		}
	}

	for client := range targetClients {
		select {
		case client.send <- message:
		default:
			h.log.Warn("ws: client channel full, force unregister", "id", client.ID)
			h.unregister <- client
		}
	}
}
