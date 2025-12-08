// Package rest
package rest

import (
	"database/sql"
	"net/http"

	"horizonx-server/api/rest/handler"
	"horizonx-server/api/rest/middleware"
	"horizonx-server/internal/config"
	"horizonx-server/internal/core/auth"
	"horizonx-server/internal/logger"
	"horizonx-server/internal/storage/snapshot"
	"horizonx-server/internal/storage/sqlite"
	"horizonx-server/internal/transport/websocket"
)

func NewRouter(cfg *config.Config, ms *snapshot.MetricsStore, hub *websocket.Hub, db *sql.DB, log logger.Logger) http.Handler {
	userRepo := sqlite.NewUserRepository(db)
	authService := auth.NewService(userRepo, cfg.JWTSecret, cfg.JWTExpiry)
	authHandler := handler.NewAuthHandler(authService, cfg)
	wsHandler := websocket.NewHandler(hub, cfg, log)
	metricsHandler := handler.NewMetricsHandler(ms)

	mux := http.NewServeMux()

	globalMw := middleware.New()
	globalMw.Use(middleware.CORS(cfg))
	globalMw.Use(middleware.CSRF(cfg))

	authMw := middleware.New()
	authMw.Use(middleware.JWT(cfg))

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	mux.HandleFunc("POST /auth/register", authHandler.Register)
	mux.HandleFunc("POST /auth/login", authHandler.Login)
	mux.HandleFunc("POST /auth/logout", authHandler.Logout)

	mux.HandleFunc("/ws", wsHandler.Serve)
	mux.Handle("GET /metrics", authMw.Then(http.HandlerFunc(metricsHandler.Get)))

	// Placeholder for new feature routes
	// mux.HandleFunc("/ssh", handler.HandleSSH)
	// mux.HandleFunc("/deploy", handler.HandleDeploy)

	return globalMw.Apply(mux)
}
