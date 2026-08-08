package agentws

import (
	"context"
	"encoding/json"

	"horizonx/internal/domain"
	"horizonx/internal/logger"

	"github.com/google/uuid"
)

// commandRequest is the send-path unit: a request to deliver an AgentCommand
// to one agent, answered by the router's Run() loop so the agents map stays
// goroutine-confined (no locks, no races).
type commandRequest struct {
	serverID uuid.UUID
	payload  json.RawMessage
	reply    chan error
}

type Router struct {
	ctx    context.Context
	cancel context.CancelFunc

	agents map[uuid.UUID]*Client

	register   chan *Client
	unregister chan *Client
	commands   chan *commandRequest

	log logger.Logger
}

func NewRouter(parent context.Context, log logger.Logger) *Router {
	ctx, cancel := context.WithCancel(parent)

	return &Router{
		ctx:        ctx,
		cancel:     cancel,
		agents:     make(map[uuid.UUID]*Client),
		register:   make(chan *Client, 64),
		unregister: make(chan *Client, 64),
		commands:   make(chan *commandRequest, 64),
		log:        log,
	}
}

func (r *Router) Run() {
	for {
		select {
		case <-r.ctx.Done():
			r.log.Info("ws: agent router shutting down...")
			for _, agent := range r.agents {
				close(agent.send)
			}
			return

		case a := <-r.register:
			r.agents[a.ID] = a
			a.log.Info("ws: agent registered", "id", a.ID)

		case a := <-r.unregister:
			agent, ok := r.agents[a.ID]
			if !ok {
				continue
			}

			delete(r.agents, a.ID)
			close(agent.send)
			r.log.Info("ws: agent unregistered", "id", a.ID)

		case req := <-r.commands:
			agent, ok := r.agents[req.serverID]
			if !ok {
				req.reply <- domain.ErrAgentOffline
				continue
			}

			// The agent's send channel is buffered (256); a full buffer
			// means its writePump is stuck — surface that as an error
			// rather than block the router loop.
			select {
			case agent.send <- req.payload:
				req.reply <- nil
			default:
				req.reply <- domain.ErrAgentOffline
			}
		}
	}
}

// SendCommand delivers an AgentCommand to the given server's live agent
// connection. It blocks until the router loop has handed the message to the
// agent's send queue (or rejected it), so callers get synchronous
// ErrAgentOffline when the agent is down — no fire-and-forget.
func (r *Router) SendCommand(ctx context.Context, serverID uuid.UUID, cmd domain.AgentCommand) error {
	cmdPayload, err := json.Marshal(cmd)
	if err != nil {
		return err
	}

	// The agent reads domain.WsServerMessage on the wire (agent/conn.go),
	// so wrap the command in that envelope.
	payload, err := json.Marshal(domain.WsServerMessage{
		TargetServerID: serverID,
		Payload:        cmdPayload,
	})
	if err != nil {
		return err
	}

	req := &commandRequest{
		serverID: serverID,
		payload:  payload,
		reply:    make(chan error, 1),
	}

	select {
	case r.commands <- req:
	case <-r.ctx.Done():
		return domain.ErrAgentOffline
	case <-ctx.Done():
		return ctx.Err()
	}

	select {
	case err := <-req.reply:
		return err
	case <-r.ctx.Done():
		return domain.ErrAgentOffline
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *Router) Stop() {
	r.cancel()
}
