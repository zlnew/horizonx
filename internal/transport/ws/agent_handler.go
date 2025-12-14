package ws

import (
	"net/http"
	"strings"

	"horizonx-server/internal/domain"
	"horizonx-server/internal/logger"

	"github.com/gorilla/websocket"
)

type AgentHandlerDeps struct {
	Server domain.ServerService
	Job    domain.JobService
}

type AgentHandler struct {
	hub      *AgentHub
	upgrader websocket.Upgrader
	log      logger.Logger
	deps     *AgentHandlerDeps
}

func NewAgentHandler(hub *AgentHub, log logger.Logger, deps *AgentHandlerDeps) *AgentHandler {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	return &AgentHandler{
		hub:      hub,
		upgrader: upgrader,
		log:      log,
		deps:     deps,
	}
}

func (h *AgentHandler) Serve(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		http.Error(w, "missing authorization header", http.StatusUnauthorized)
		return
	}

	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	serverID, secret, err := domain.ValidateAgentCredentials(parts[1])
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	if _, err := h.deps.Server.AuthorizeAgent(r.Context(), serverID, secret); err != nil {
		h.log.Warn("ws auth: invalid agent credentials")
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Error("ws auth: agent upgrade failed", "error", err)
		return
	}

	a := NewAgent(h.hub, conn, h.log, serverID)
	a.hub.register <- a

	go a.writePump()
	go a.readPump()
}
