package ws

import (
	"net/http"
	"strings"

	"horizonx-server/internal/domain"
	"horizonx-server/internal/logger"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type AgentHandler struct {
	hub      *Hub
	upgrader websocket.Upgrader
	log      logger.Logger

	serverService domain.ServerService
}

func NewAgentHandler(hub *Hub, log logger.Logger, serverService domain.ServerService) *AgentHandler {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	return &AgentHandler{
		hub:      hub,
		upgrader: upgrader,
		log:      log,

		serverService: serverService,
	}
}

func (h *AgentHandler) Serve(w http.ResponseWriter, r *http.Request) {
	var clientID string
	var clientType string

	auth := r.Header.Get("Authorization")
	if auth == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	raw := strings.TrimSpace(parts[1])

	tokenParts := strings.SplitN(raw, ".", 2)
	if len(tokenParts) != 2 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	rawServerID := tokenParts[0]
	secret := tokenParts[1]

	serverID, err := uuid.Parse(rawServerID)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if secret == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	server, err := h.serverService.AuthorizeAgent(r.Context(), serverID, secret)
	if err != nil {
		h.log.Warn("ws auth: invalid credentials")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	clientID = server.ID.String()
	clientType = domain.WsClientAgent

	if clientID == "" {
		h.log.Warn("ws auth: invalid credentials")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Error("ws auth: upgrade failed", "error", err)
		return
	}

	c := NewClient(h.hub, conn, h.log, clientID, clientType)
	c.hub.register <- c

	go c.writePump()
	go c.readPump()
}
