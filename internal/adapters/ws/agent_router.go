package ws

import (
	"context"
	"encoding/json"

	"horizonx-server/internal/domain"
	"horizonx-server/internal/logger"

	"github.com/google/uuid"
)

type AgentRouter struct {
	ctx    context.Context
	cancel context.CancelFunc

	agents map[uuid.UUID]*Agent

	register   chan *Agent
	unregister chan *Agent
	commands   chan *domain.WsAgentCommand

	log logger.Logger
}

func NewAgentRouter(parent context.Context, log logger.Logger) *AgentRouter {
	ctx, cancel := context.WithCancel(parent)

	return &AgentRouter{
		ctx:        ctx,
		cancel:     cancel,
		agents:     make(map[uuid.UUID]*Agent),
		register:   make(chan *Agent, 64),
		unregister: make(chan *Agent, 64),
		commands:   make(chan *domain.WsAgentCommand, 1024),
		log:        log,
	}
}

func (h *AgentRouter) Run() {
	for {
		select {
		case <-h.ctx.Done():
			h.log.Info("ws: agent hub shutting down...")
			for _, agent := range h.agents {
				close(agent.send)
			}
			return

		case a := <-h.register:
			h.agents[a.ID] = a
			a.log.Info("ws: agent registered", "id", a.ID)

		case a := <-h.unregister:
			agent, ok := h.agents[a.ID]
			if !ok {
				continue
			}

			delete(h.agents, a.ID)
			close(agent.send)
			h.log.Info("ws: agent unregistered", "id", a.ID)

		case cmd := <-h.commands:
			h.handleCommand(cmd)
		}
	}
}

func (h *AgentRouter) Stop() {
	h.cancel()
}

func (h *AgentRouter) SendCommand(cmd *domain.WsAgentCommand) {
	select {
	case h.commands <- cmd:
	case <-h.ctx.Done():
	default:
		h.log.Warn("ws: command buffer full, dropping command", "command", cmd.CommandType)
	}
}

func (h *AgentRouter) handleCommand(cmd *domain.WsAgentCommand) {
	agent, ok := h.agents[cmd.TargetServerID]
	if !ok {
		h.log.Warn("ws: target agent not connected", "server_id", cmd.TargetServerID)
		return
	}

	message, err := json.Marshal(cmd)
	if err != nil {
		h.log.Error("ws: failed to marshal server command", "error", err)
		return
	}

	select {
	case agent.send <- message:
		h.log.Info("ws: command sent to agent", "server_id", agent.ID, "command", cmd.CommandType)
	default:
		h.log.Warn("ws: agent channel full, force unregister", "server_id", agent.ID)
		h.unregister <- agent
	}
}
