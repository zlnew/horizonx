package userws

import (
	"fmt"
	"net/http"
	"slices"

	"horizonx/internal/domain"
	"horizonx/internal/logger"

	"github.com/gorilla/websocket"
)

type Handler struct {
	hub      *Hub
	upgrader websocket.Upgrader
	log      logger.Logger

	secret         string
	allowedOrigins []string
}

func NewHandler(hub *Hub, log logger.Logger, secret string, allowedOrigins []string) *Handler {
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

	return &Handler{
		hub:      hub,
		upgrader: upgrader,
		log:      log,

		secret:         secret,
		allowedOrigins: allowedOrigins,
	}
}

func (h *Handler) Serve(w http.ResponseWriter, r *http.Request) {
	var clientID string

	cookie, err := r.Cookie("horizonx_access_token")
	if err == nil {
		tokenString := cookie.Value
		claims, err := domain.ValidateToken(tokenString, h.secret)
		if err == nil {
			clientID = fmt.Sprintf("%v", claims.UserID)
		}
	}

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

	c := NewClient(h.hub, conn, h.log, clientID)
	c.hub.register <- c

	go c.writePump()
	go c.readPump()
}
