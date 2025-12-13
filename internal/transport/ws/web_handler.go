package ws

import (
	"fmt"
	"net/http"
	"slices"

	"horizonx-server/internal/domain"
	"horizonx-server/internal/logger"
	"horizonx-server/pkg"

	"github.com/gorilla/websocket"
)

type WebHandler struct {
	hub      *Hub
	upgrader websocket.Upgrader
	log      logger.Logger

	secret         string
	allowedOrigins []string
}

func NewWebHandler(hub *Hub, log logger.Logger, secret string, allowedOrigins []string) *WebHandler {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true
			}

			allowed := slices.Contains(allowedOrigins, origin)
			if !allowed {
				log.Warn("ws auth: origin rejected", "origin", origin)
				return false
			}

			return allowed
		},
	}

	return &WebHandler{
		hub:      hub,
		upgrader: upgrader,
		log:      log,

		secret:         secret,
		allowedOrigins: allowedOrigins,
	}
}

func (h *WebHandler) Serve(w http.ResponseWriter, r *http.Request) {
	var clientID string
	var clientType string

	cookie, err := r.Cookie("access_token")
	if err == nil {
		tokenString := cookie.Value
		claims, err := pkg.ValidateToken(tokenString, h.secret)
		if err == nil {
			if sub, ok := claims["sub"]; ok && sub != nil {
				clientID = fmt.Sprintf("%v", sub)
				clientType = domain.WsClientUser
			}
		}
	}

	if clientID == "" {
		h.log.Warn("ws auth: no valid credentials")
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
