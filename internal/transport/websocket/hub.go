// Package websocket
package websocket

import (
	"encoding/json"

	"horizonx-server/internal/logger"
)

type Hub struct {
	rooms  map[string]map[*Client]bool
	agents map[string]*Client

	register    chan *Client
	unregister  chan *Client
	subscribe   chan *Subscription
	unsubscribe chan *Subscription

	events   chan *ServerEvent
	commands chan *CommandEvent

	log logger.Logger
}

type Subscription struct {
	client  *Client
	channel string
}

type ServerEvent struct {
	Channel string
	Event   string
	Payload any
}

type CommandEvent struct {
	TargetServerID string
	CommandType    string
	Payload        any
}

func NewHub(log logger.Logger) *Hub {
	return &Hub{
		rooms:       make(map[string]map[*Client]bool),
		agents:      make(map[string]*Client),
		register:    make(chan *Client),
		unregister:  make(chan *Client),
		subscribe:   make(chan *Subscription),
		unsubscribe: make(chan *Subscription),
		events:      make(chan *ServerEvent, 100),
		commands:    make(chan *CommandEvent, 100),
		log:         log,
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			if client.Type == TypeAgent {
				h.agents[client.ID] = client
				h.log.Info("agent online", "server_id", client.ID)
			}

		case client := <-h.unregister:
			if client.Type == TypeAgent {
				if _, ok := h.agents[client.ID]; ok {
					delete(h.agents, client.ID)
					h.log.Info("agent offline", "server_id", client.ID)
				}
			}
			for roomName, clients := range h.rooms {
				if _, ok := clients[client]; ok {
					delete(clients, client)
					if len(clients) == 0 {
						delete(h.rooms, roomName)
					}
				}
			}

		case sub := <-h.subscribe:
			if _, ok := h.rooms[sub.channel]; !ok {
				h.rooms[sub.channel] = make(map[*Client]bool)
			}
			h.rooms[sub.channel][sub.client] = true
			h.log.Debug("client subscribed", "channel", sub.channel)

		case sub := <-h.unsubscribe:
			if clients, ok := h.rooms[sub.channel]; ok {
				delete(clients, sub.client)
				if len(clients) == 0 {
					delete(h.rooms, sub.channel)
				}
			}

		case evt := <-h.events:
			data := map[string]any{
				"type":    "event",
				"event":   evt.Event,
				"channel": evt.Channel,
				"payload": evt.Payload,
			}
			bytes, _ := json.Marshal(data)

			if clients, ok := h.rooms[evt.Channel]; ok {
				for client := range clients {
					select {
					case client.send <- bytes:
					default:
					}
				}
			}

		case cmd := <-h.commands:
			agentClient, ok := h.agents[cmd.TargetServerID]
			if !ok {
				h.log.Warn("cannot send command: agent offline", "target_id", cmd.TargetServerID)
				continue
			}

			payload := map[string]any{
				"type":    "command",
				"command": cmd.CommandType,
				"payload": cmd.Payload,
			}
			bytes, _ := json.Marshal(payload)

			select {
			case agentClient.send <- bytes:
				h.log.Info("command sent to agent", "target_id", cmd.TargetServerID, "cmd", cmd.CommandType)
			default:
				h.log.Error("agent send buffer full", "target_id", cmd.TargetServerID)
			}
		}
	}
}

func (h *Hub) Emit(channel, event string, payload any) {
	h.events <- &ServerEvent{
		Channel: channel,
		Event:   event,
		Payload: payload,
	}
}

func (h *Hub) SendCommand(serverID, cmdType string, payload any) error {
	h.commands <- &CommandEvent{
		TargetServerID: serverID,
		CommandType:    cmdType,
		Payload:        payload,
	}

	return nil
}
