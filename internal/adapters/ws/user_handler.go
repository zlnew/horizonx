package ws

import (
	"fmt"
	"net/http"
	"slices"

	"horizonx-server/internal/logger"
	"horizonx-server/internal/pkg"

	"github.com/gorilla/websocket"
)

type UserHandler struct {
	hub      *Hub
	upgrader websocket.Upgrader
	log      logger.Logger

	secret         string
	allowedOrigins []string
}

func NewUserHandler(hub *Hub, log logger.Logger, secret string, allowedOrigins []string) *UserHandler {
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

	return &UserHandler{
		hub:      hub,
		upgrader: upgrader,
		log:      log,

		secret:         secret,
		allowedOrigins: allowedOrigins,
	}
}

func (h *UserHandler) Serve(w http.ResponseWriter, r *http.Request) {
	var clientID string

	cookie, err := r.Cookie("access_token")
	if err == nil {
		tokenString := cookie.Value
		claims, err := pkg.ValidateToken(tokenString, h.secret)
		if err == nil {
			if sub, ok := claims["sub"]; ok && sub != nil {
				clientID = fmt.Sprintf("%v", sub)
			}
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

	c := NewUser(h.hub, conn, h.log, clientID)
	c.hub.register <- c

	go c.writePump()
	go c.readPump()
}
