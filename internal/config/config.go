// Package config
package config

import (
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

type Config struct {
	Address        string
	AllowedOrigins []string
	DatabaseURL    string
	JWTSecret      string
	JWTExpiry      time.Duration
	LogLevel       string
	LogFormat      string

	AgentTargetAPIURL   string
	AgentTargetWsURL    string
	AgentServerAPIToken string
	AgentServerID       uuid.UUID
}

func Load() *Config {
	_ = godotenv.Load()

	// Logs
	logLevel := getEnv("LOG_LEVEL", "info")
	logFormat := getEnv("LOG_FORMAT", "text")

	// Server HTTP Address
	addr := getEnv("HTTP_ADDR", ":3000")

	// Server Allowed Origins
	var origins []string
	rawOrigins := os.Getenv("ALLOWED_ORIGINS")
	if rawOrigins != "" {
		parts := strings.SplitSeq(rawOrigins, ",")
		for o := range parts {
			if trimmed := strings.TrimSpace(o); trimmed != "" {
				origins = append(origins, trimmed)
			}
		}
	}

	// Database URL
	databaseURL := getEnv("DATABASE_URL", "postgres://user:pass@localhost:5432/horizonx")

	// JWT Secret and Expiry
	jwtSecret := getEnv("JWT_SECRET", "")
	jwtExpiry := 24 * time.Hour
	if raw := os.Getenv("JWT_EXPIRY"); raw != "" {
		if duration, err := time.ParseDuration(raw); err == nil && duration > 0 {
			jwtExpiry = duration
		}
	}

	// AGENT Target URL
	agentTargetAPIURL := getEnv("HORIZONX_API_URL", "http://localhost:3000")
	agentTargetWsURL := getEnv("HORIZONX_WS_URL", "ws://localhost:3000/ws/agent")

	// AGENT Server Credentials
	agentServerAPIToken := getEnv("HORIZONX_SERVER_API_TOKEN", "")
	var agentServerID uuid.UUID
	if raw := os.Getenv("HORIZONX_SERVER_ID"); raw != "" {
		if serverID, err := uuid.Parse(raw); err == nil {
			agentServerID = serverID
		}
	}

	return &Config{
		LogLevel:  logLevel,
		LogFormat: logFormat,

		Address:        addr,
		AllowedOrigins: origins,
		DatabaseURL:    databaseURL,
		JWTSecret:      jwtSecret,
		JWTExpiry:      jwtExpiry,

		AgentTargetAPIURL:   agentTargetAPIURL,
		AgentTargetWsURL:    agentTargetWsURL,
		AgentServerAPIToken: agentServerAPIToken,
		AgentServerID:       agentServerID,
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
