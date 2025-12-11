package websocket

import (
	"fmt"
	"net/http"
	"slices"

	"horizonx-server/internal/config"
	"horizonx-server/internal/domain"
	"horizonx-server/internal/logger"
	"horizonx-server/pkg"

	"github.com/gorilla/websocket"
)

type Handler struct {
	hub      *Hub
	upgrader websocket.Upgrader
	cfg      *config.Config
	log      logger.Logger

	serverService domain.ServerService
}

func NewHandler(hub *Hub, cfg *config.Config, log logger.Logger, serverService domain.ServerService) *Handler {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true
			}

			allowed := slices.Contains(cfg.AllowedOrigins, origin)
			if !allowed {
				log.Warn("ws origin rejected", "origin", origin)
				return false
			}

			return allowed
		},
	}

	return &Handler{
		hub:           hub,
		upgrader:      upgrader,
		cfg:           cfg,
		log:           log,
		serverService: serverService,
	}
}

func (h *Handler) Serve(w http.ResponseWriter, r *http.Request) {
	var clientID string
	var clientType string

	// Authorize for browser
	cookie, err := r.Cookie("access_token")
	if err == nil {
		tokenString := cookie.Value
		claims, err := pkg.ValidateToken(tokenString, h.cfg.JWTSecret)
		if err == nil {
			if sub, ok := claims["sub"]; ok && sub != nil {
				clientID = fmt.Sprintf("%v", sub)
				clientType = TypeUser
				h.log.Debug("ws auth: user authenticated", "id", clientID)
			} else {
				h.log.Warn("ws auth: sub claim missing or nil")
			}
		} else {
			h.log.Warn("ws auth: invalid token", "err", err)
		}
	} else {
		h.log.Debug("ws auth: no access_token cookie found")
	}

	// Authorize for agents
	if clientID == "" {
		token := ""

		authHeader := r.Header.Get("Authorization")
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token = authHeader[7:]
		}

		if token != "" {
			server, err := h.serverService.AuthorizeAgent(r.Context(), token)
			if err == nil {
				clientID = fmt.Sprintf("%d", server.ID)
				clientType = TypeAgent
				h.log.Debug("ws auth: agent authenticated", "server_id", clientID)
			} else {
				h.log.Warn("ws auth: invalid agent token")
			}
		}
	}

	if clientID == "" {
		h.log.Warn("ws unauthorized: no valid credentials")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Error("ws upgrade failed", "error", err)
		return
	}

	client := NewClient(h.hub, conn, h.log, clientID, clientType)

	h.hub.register <- client

	go client.writePump()
	go client.readPump()

	h.log.Info("ws client connected", "remote_addr", conn.RemoteAddr())
}
