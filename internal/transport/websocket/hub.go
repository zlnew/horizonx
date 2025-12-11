// Package websocket
package websocket

import (
	"encoding/json"
	"fmt"
	"strings"

	"horizonx-server/internal/core/metrics"
	"horizonx-server/internal/domain"
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

	serverService  domain.ServerService
	metricsService *metrics.Service

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

func NewHub(log logger.Logger, serverService domain.ServerService, metricsService *metrics.Service) *Hub {
	return &Hub{
		rooms:          make(map[string]map[*Client]bool),
		agents:         make(map[string]*Client),
		register:       make(chan *Client),
		unregister:     make(chan *Client),
		subscribe:      make(chan *Subscription),
		unsubscribe:    make(chan *Subscription),
		events:         make(chan *ServerEvent, 100),
		commands:       make(chan *CommandEvent, 100),
		serverService:  serverService,
		metricsService: metricsService,
		log:            log,
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			if client.Type == TypeAgent {
				h.agents[client.ID] = client
				h.initAgent(client.ID, client)
			}

		case client := <-h.unregister:
			if client.Type == TypeAgent {
				if currentAgent, ok := h.agents[client.ID]; ok && currentAgent == client {
					delete(h.agents, client.ID)
					go h.updateAgentServerStatus(client.ID, false)
					h.log.Info("ws: agent offline", "server_id", client.ID)
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
			h.log.Debug("ws: client subscribed", "channel", sub.channel)

		case sub := <-h.unsubscribe:
			if clients, ok := h.rooms[sub.channel]; ok {
				delete(clients, sub.client)
				if len(clients) == 0 {
					delete(h.rooms, sub.channel)
				}
			}

		case evt := <-h.events:
			if h.metricsService != nil && strings.HasSuffix(evt.Channel, ":metrics") && evt.Event == domain.EventServerMetricsReport {
				rawJSON, ok := evt.Payload.(json.RawMessage)
				if !ok {
					h.log.Error("metric payload is not json.RawMessage", "type", fmt.Sprintf("%T", evt.Payload))
					continue
				}

				var m domain.Metrics
				if err := json.Unmarshal(rawJSON, &m); err != nil {
					h.log.Error("failed to unmarshal domain.Metrics payload in hub", "error", err)
					continue
				}

				if err := h.metricsService.Ingest(m); err != nil {
					h.log.Error("failed to process ingested metric from hub", "error", err)
				}

				h.Emit(evt.Channel, domain.EventServerMetricsReceived, m)
				continue
			}

			h.log.Debug("ws: processing event for broadcast", "channel", evt.Channel, "event", evt.Event)
			data := map[string]any{
				"type":    "event",
				"event":   evt.Event,
				"channel": evt.Channel,
				"payload": evt.Payload,
			}
			bytes, err := json.Marshal(data)
			if err != nil {
				h.log.Error("ws: failed to marshal event for broadcast", "error", err)
				continue
			}

			if clients, ok := h.rooms[evt.Channel]; ok {
				h.log.Debug("ws: found room for channel", "channel", evt.Channel, "clients_count", len(clients))
				for client := range clients {
					select {
					case client.send <- bytes:
						h.log.Debug("ws: sent event to client", "channel", evt.Channel, "client_id", client.ID, "client_type", client.Type)
					default:
						h.log.Warn("ws: DROPPED event, client send buffer full", "channel", evt.Channel, "client_id", client.ID)
					}
				}
			} else {
				h.log.Warn("ws: no room found for channel, event not broadcasted", "channel", evt.Channel)
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

func (h *Hub) Events() <-chan *ServerEvent {
	return h.events
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
