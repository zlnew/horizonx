package agentws

import (
	"net/http"
	"strings"

	"horizonx/internal/domain"
	"horizonx/internal/logger"

	"github.com/gorilla/websocket"
)

type Handler struct {
	router   *Router
	upgrader websocket.Upgrader
	log      logger.Logger
	svc      domain.ServerService

	// publish forwards events onto the server event bus (e.g.
	// container_log_chunk → userws subscriber → app_logs:{appID} channel).
	publish func(eventName string, event any)
}

func NewHandler(router *Router, log logger.Logger, svc domain.ServerService) *Handler {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	return &Handler{
		router:   router,
		upgrader: upgrader,
		log:      log,
		svc:      svc,
	}
}

// SetPublisher wires the event-bus callback (called before Serve; nil is a
// safe no-op — events just don't fan out).
func (h *Handler) SetPublisher(publish func(eventName string, event any)) {
	h.publish = publish
}

func (h *Handler) Serve(w http.ResponseWriter, r *http.Request) {
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

	if _, err := h.svc.AuthorizeAgent(r.Context(), serverID, secret); err != nil {
		h.log.Warn("ws auth: invalid agent credentials")
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Error("ws auth: agent upgrade failed", "error", err)
		return
	}

	if err := h.svc.UpdateStatus(r.Context(), serverID, true); err != nil {
		h.log.Error("ws: failed to set server online", "server_id", serverID.String())
		_ = conn.Close()
		return
	}

	a := NewClient(h.router, conn, h.log, h.svc, serverID)
	a.publish = h.publish
	a.hub.register <- a

	go a.writePump()
	go a.readPump()
}
