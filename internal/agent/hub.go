package agent

import (
	"context"
	"encoding/json"
	"strconv"

	"horizonx-server/internal/domain"
	"horizonx-server/internal/logger"
)

type Hub struct {
	ctx    context.Context
	cancel context.CancelFunc

	agent        *Agent
	agentEvents  chan *domain.WsInternalEvent
	serverEvents chan *domain.WsClientMessage
	commands     chan *domain.WsAgentCommand
	log          logger.Logger
}

func NewHub(parent context.Context, log logger.Logger) *Hub {
	ctx, cancel := context.WithCancel(parent)

	return &Hub{
		ctx:          ctx,
		cancel:       cancel,
		agentEvents:  make(chan *domain.WsInternalEvent, 16),
		serverEvents: make(chan *domain.WsClientMessage, 64),
		commands:     make(chan *domain.WsAgentCommand, 64),
		log:          log,
	}
}

func (h *Hub) SetAgent(a *Agent) {
	h.agent = a
}

func (h *Hub) Run() {
	for {
		select {
		case <-h.ctx.Done():
			h.log.Info("hub shutting down...")
			return
		case cmd := <-h.commands:
			h.handleCommand(cmd)
		case ev := <-h.serverEvents:
			h.handleServerEvent(ev)
		}
	}
}

func (h *Hub) Stop() {
	h.cancel()
}

func (h *Hub) BroadcastToServer(ev *domain.WsClientMessage) {
	select {
	case <-h.ctx.Done():
		return
	case h.serverEvents <- ev:
	}
}

func (h *Hub) SendToAgent(ev *domain.WsInternalEvent) {
	select {
	case <-h.ctx.Done():
		return
	case h.agentEvents <- ev:
	default:
		h.log.Warn("agent events buffer full, dropping internal event", "event", ev.Event)
	}
}

func (h *Hub) handleServerEvent(ev *domain.WsClientMessage) {
	if h.agent == nil {
		h.log.Warn("no agent attached to hub, dropping server event", "event", ev.Event)
		return
	}

	msg, err := json.Marshal(ev)
	if err != nil {
		h.log.Error("failed to marshal server event", "error", err)
		return
	}

	select {
	case <-h.ctx.Done():
		return
	case h.agent.send <- msg:
	default:
		h.log.Warn("agent send buffer full, dropping server event", "event", ev.Event)
	}
}

func (h *Hub) handleCommand(cmd *domain.WsAgentCommand) {
	h.log.Info("executing command", "type", cmd.CommandType)

	switch cmd.CommandType {
	case "init":
		serverID, err := strconv.ParseInt(cmd.TargetServerID, 10, 64)
		if err != nil {
			h.log.Error("failed to parse server id to int64", "target", cmd.TargetServerID, "error", err)
			return
		}

		h.SendToAgent(&domain.WsInternalEvent{
			Event:   domain.WsEventAgentReady,
			Payload: serverID,
		})

		h.BroadcastToServer(&domain.WsClientMessage{
			Type:  domain.WsAgentReport,
			Event: domain.WsEventAgentReady,
		})
	default:
		h.log.Warn("unknown command received", "type", cmd.CommandType)
	}
}
