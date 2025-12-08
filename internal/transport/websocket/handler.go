package websocket

import (
	"net/http"
	"slices"

	"horizonx-server/internal/config"
	"horizonx-server/internal/core/auth"
	"horizonx-server/internal/logger"

	"github.com/gorilla/websocket"
)

type Handler struct {
	hub      *Hub
	upgrader websocket.Upgrader
	cfg      *config.Config
	log      logger.Logger
}

func NewHandler(hub *Hub, cfg *config.Config, log logger.Logger) *Handler {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true
			}

			allowed := slices.Contains(cfg.AllowedOrigins, origin)
			if !allowed {
				log.Warn("websocket origin rejected", "origin", origin)
				return false
			}

			return allowed
		},
	}

	return &Handler{
		hub:      hub,
		upgrader: upgrader,
		cfg:      cfg,
		log:      log,
	}
}

func (h *Handler) Serve(w http.ResponseWriter, r *http.Request) {
	var tokenString string

	if cookie, err := r.Cookie("access_token"); err == nil {
		tokenString = cookie.Value
	}

	if tokenString == "" {
		h.log.Warn("websocket unauthorized: no token found")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	claims, err := auth.ValidateToken(tokenString, h.cfg.JWTSecret)
	if err != nil {
		h.log.Warn("websocket jwt verification failed", "error", err)
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Error("websocket upgrade failed", "error", err)
		return
	}

	userID := claims["sub"]
	h.log.Info("ws client authenticated", "user_id", userID)

	client := NewClient(h.hub, conn, h.log)
	go client.writePump()
	go client.readPump()

	h.log.Info("ws client connected", "remote_addr", conn.RemoteAddr())
}
