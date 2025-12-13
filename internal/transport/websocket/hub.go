// Package websocket
package websocket

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"horizonx-server/internal/domain"
	"horizonx-server/internal/logger"
)

type Hub struct {
	ctx    context.Context
	cancel context.CancelFunc

	clients  map[*Client]bool
	agents   map[string]*Client
	channels map[string]map[*Client]bool

	register    chan *Client
	unregister  chan *Client
	subscribe   chan *Subscription
	unsubscribe chan *Subscription
	agentReady  chan *Client

	events   chan *domain.WsInternalEvent
	commands chan *domain.WsAgentCommand

	serverService  domain.ServerService
	metricsService domain.MetricsService

	log logger.Logger
}

type Subscription struct {
	client  *Client
	channel string
}

func NewHub(parent context.Context, log logger.Logger, serverService domain.ServerService, metricsService domain.MetricsService) *Hub {
	ctx, cancel := context.WithCancel(parent)

	return &Hub{
		ctx:    ctx,
		cancel: cancel,

		clients:  make(map[*Client]bool),
		agents:   make(map[string]*Client),
		channels: make(map[string]map[*Client]bool),

		register:    make(chan *Client),
		unregister:  make(chan *Client),
		subscribe:   make(chan *Subscription),
		unsubscribe: make(chan *Subscription),
		agentReady:  make(chan *Client),

		events:   make(chan *domain.WsInternalEvent, 100),
		commands: make(chan *domain.WsAgentCommand, 100),

		serverService:  serverService,
		metricsService: metricsService,
		log:            log,
	}
}

func (h *Hub) Run() {
	for {
		select {
		case <-h.ctx.Done():
			h.log.Info("ws: hub shutting down")
			for client := range h.clients {
				close(client.send)
			}
			return

		case client := <-h.register:
			h.clients[client] = true
			h.log.Info("ws: client registered", "id", client.ID, "type", client.Type, "total_clients", len(h.clients))

			if client.Type == domain.WsClientAgent {
				h.agents[client.ID] = client
				h.SendCommand(&domain.WsAgentCommand{
					TargetServerID: client.ID,
					CommandType:    "init",
				})

				h.log.Info("ws: agent registered", "server_id", client.ID, "total_agents", len(h.agents))
			}

		case client := <-h.unregister:
			if !h.clients[client] {
				continue
			}

			delete(h.clients, client)
			close(client.send)
			h.log.Info("ws: client unregistered", "id", client.ID, "type", client.Type, "total_clients", len(h.clients))

			if client.Type == domain.WsClientAgent {
				if _, agentOk := h.agents[client.ID]; agentOk {
					delete(h.agents, client.ID)
					go h.updateAgentServerStatus(client.ID, false)
					h.log.Info("ws: agent unregistered", "server_id", client.ID, "total_agents", len(h.agents))
				}
			}

			for channelID, subs := range h.channels {
				if _, subscribed := subs[client]; subscribed {
					delete(subs, client)
					if len(subs) == 0 {
						delete(h.channels, channelID)
					}
				}
			}

		case sub := <-h.subscribe:
			if h.channels[sub.channel] == nil {
				h.channels[sub.channel] = make(map[*Client]bool)
			}
			h.channels[sub.channel][sub.client] = true
			h.log.Debug(
				"ws: client subscribed",
				"client_id", sub.client.ID,
				"client_type", sub.client.Type,
				"channel", sub.channel,
			)

		case sub := <-h.unsubscribe:
			if subs, ok := h.channels[sub.channel]; ok {
				if _, subscribed := subs[sub.client]; subscribed {
					delete(subs, sub.client)
					if len(subs) == 0 {
						delete(h.channels, sub.channel)
					}
					h.log.Debug(
						"ws: client unsubscribed",
						"client_id", sub.client.ID,
						"client_type", sub.client.Type,
						"channel", sub.channel,
					)
				}
			}

		case client := <-h.agentReady:
			if client.Type == domain.WsClientAgent {
				h.log.Info("ws: agent is now fully operational", "server_id", client.ID)
				go h.updateAgentServerStatus(client.ID, true)
			}

		case event := <-h.events:
			h.handleEvent(event)

		case command := <-h.commands:
			h.handleCommand(command)
		}
	}
}

func (h *Hub) Stop() {
	h.cancel()
}

func (h *Hub) handleEvent(event *domain.WsInternalEvent) {
	if h.metricsService != nil && strings.HasSuffix(event.Channel, ":metrics") && event.Event == domain.WsEventServerMetricsReport {
		rawJSON, ok := event.Payload.(json.RawMessage)
		if !ok {
			h.log.Error("ws: invalid metrics payload")
			return
		}

		var m domain.Metrics
		if err := json.Unmarshal(rawJSON, &m); err != nil {
			h.log.Error("ws: invalid to process metrics payload", "error", err)
			return
		}

		if err := h.metricsService.Ingest(m); err != nil {
			h.log.Error("ws: failed to process ingested metrics", "error", err)
			return
		}

		h.Broadcast(&domain.WsInternalEvent{
			Channel: event.Channel,
			Event:   domain.WsEventServerMetricsReceived,
			Payload: m,
		})

		return
	}

	message, err := json.Marshal(event)
	if err != nil {
		h.log.Error("ws: failed to marshal server event", "error", err)
		return
	}

	targetClients := h.clients

	if event.Channel != "" {
		if subs, ok := h.channels[event.Channel]; ok {
			targetClients = subs
		} else {
			h.log.Debug("ws: event channels has no subscribers", "channel", event.Channel)
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

func (h *Hub) handleCommand(command *domain.WsAgentCommand) {
	agent, ok := h.agents[command.TargetServerID]
	if !ok {
		h.log.Warn("ws: cannot send command, agent offline", "server_id", command.TargetServerID)
		return
	}

	message, err := json.Marshal(command)
	if err != nil {
		h.log.Error("ws: failed to marshal command", "error", err)
		return
	}

	select {
	case agent.send <- message:
	default:
		h.log.Warn("ws: agent send buffer full", "server_id", command.TargetServerID)
	}
}

func (h *Hub) updateAgentServerStatus(serverID string, isOnline bool) {
	ctx, cancel := context.WithTimeout(h.ctx, 5*time.Second)
	defer cancel()

	parsedID, err := strconv.ParseInt(serverID, 10, 64)
	if err != nil {
		h.log.Error("ws: failed to parse server ID for status update", "id", serverID, "error", err)
		return
	}

	err = h.serverService.UpdateStatus(ctx, parsedID, isOnline)
	if err != nil {
		h.log.Error("ws: failed to update agent server status", "error", err, "server_id", parsedID, "online", isOnline)
	}

	select {
	case h.events <- &domain.WsInternalEvent{
		Channel: domain.WsChannelServerStatus,
		Event:   domain.WsEventServerStatusUpdated,
		Payload: domain.ServerStatusPayload{
			ServerID: parsedID,
			IsOnline: isOnline,
		},
	}:
	case <-h.ctx.Done():
	}
}

func (h *Hub) Broadcast(ev *domain.WsInternalEvent) {
	h.events <- ev
}

func (h *Hub) SendCommand(cmd *domain.WsAgentCommand) {
	h.commands <- cmd
}
