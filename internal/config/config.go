// Package config
package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv string

	LogLevel  string
	LogFormat string

	TimeZone       *time.Location
	Address        string
	AllowedOrigins []string
	DatabaseURL    string
	JWTSecret      string
	JWTExpiry      time.Duration

	AgentTargetAPIURL   string
	AgentTargetWsURL    string
	AgentServerAPIToken string
	AgentServerID       uuid.UUID
	AgentJobWorkerCount int

	RedisAddress  string
	RedisUsername string
	RedisPassword string
	RedisDB       int
}

func Load() *Config {
	_ = godotenv.Load()

	appEnv := getEnv("APP_ENV", "local")

	// Logs
	logLevel := getEnv("LOG_LEVEL", "info")
	logFormat := getEnv("LOG_FORMAT", "text")

	// Server Time Zone
	timeZone, err := time.LoadLocation(getEnv("TIME_ZONE", "Local"))
	if err != nil {
		timeZone = time.Local
	}

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

	agentJobWorkerCount := 10
	if raw := os.Getenv("AGENT_JOB_WORKER_COUNT"); raw != "" {
		if count, err := strconv.Atoi(raw); err == nil {
			agentJobWorkerCount = count
		}
	}

	// REDIS
	redisAddress := getEnv("REDIS_ADDR", "localhost:6379")
	redisUsername := getEnv("REDIS_USERNAME", "")
	redisPassword := getEnv("REDIS_PASS", "")
	redisDB := 0
	if raw := os.Getenv("REDIS_DB"); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil {
			redisDB = value
		}
	}

	return &Config{
		AppEnv: appEnv,

		LogLevel:  logLevel,
		LogFormat: logFormat,

		TimeZone:       timeZone,
		Address:        addr,
		AllowedOrigins: origins,
		DatabaseURL:    databaseURL,
		JWTSecret:      jwtSecret,
		JWTExpiry:      jwtExpiry,

		AgentTargetAPIURL:   agentTargetAPIURL,
		AgentTargetWsURL:    agentTargetWsURL,
		AgentServerAPIToken: agentServerAPIToken,
		AgentServerID:       agentServerID,
		AgentJobWorkerCount: agentJobWorkerCount,

		RedisAddress:  redisAddress,
		RedisUsername: redisUsername,
		RedisPassword: redisPassword,
		RedisDB:       redisDB,
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
