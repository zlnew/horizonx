// Package config
package config

import (
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Address        string
	AllowedOrigins []string
	Interval       time.Duration
	LogLevel       string
	LogFormat      string
	JWTSecret      string
	JWTExpiry      time.Duration
	DBPath         string
}

func Load() *Config {
	godotenv.Load()

	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":3000"
	}

	var origins []string
	rawOrigins := os.Getenv("ALLOWED_ORIGINS")
	if rawOrigins != "" {
		for o := range strings.SplitSeq(rawOrigins, ",") {
			o = strings.TrimSpace(o)
			if o != "" {
				origins = append(origins, o)
			}
		}
	}

	interval := 3 * time.Second
	if raw := os.Getenv("SCRAPE_INTERVAL"); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 {
			interval = parsed
		}
	}

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}

	logFormat := os.Getenv("LOG_FORMAT")
	if logFormat == "" {
		logFormat = "text"
	}

	jwtSecret := os.Getenv("JWT_SECRET")

	jwtExpiry := 24 * time.Hour
	if raw := os.Getenv("JWT_EXPIRY"); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 {
			jwtExpiry = parsed
		}
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./app.db"
	}

	return &Config{
		Address:        addr,
		AllowedOrigins: origins,
		Interval:       interval,
		LogLevel:       logLevel,
		LogFormat:      logFormat,
		JWTSecret:      jwtSecret,
		JWTExpiry:      jwtExpiry,
		DBPath:         dbPath,
	}
}
