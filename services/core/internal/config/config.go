// Package config loads service configuration from environment variables.
package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Addr          string
	NetworkFile   string
	DatabaseURL   string // empty => in-memory store
	RedisAddr     string // empty => Redis disabled
	MLURL         string // empty => heuristic fallbacks only
	MLTimeout     time.Duration
	JWTSecret     string
	TicketSecret  string
	IngestKey     string
	AdminEmail    string
	AdminPassword string
	CORSOrigins   string
	SimSpeed      float64 // simulated minutes per real minute
	TickInterval  time.Duration
	Timezone      string
	RateLimitRPS  float64 // per client IP; 0 disables (e.g. for load tests)
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func Load() Config {
	speed, err := strconv.ParseFloat(env("SIM_SPEED", "1"), 64)
	if err != nil || speed <= 0 {
		speed = 1
	}
	mlTimeoutMs, _ := strconv.Atoi(env("ML_TIMEOUT_MS", "800"))
	tickMs, _ := strconv.Atoi(env("TICK_MS", "2000"))
	rps, err := strconv.ParseFloat(env("RATE_LIMIT_RPS", "20"), 64)
	if err != nil || rps < 0 {
		rps = 20
	}
	if tickMs < 200 {
		tickMs = 2000
	}
	return Config{
		Addr:          env("ADDR", ":8080"),
		NetworkFile:   env("NETWORK_FILE", "../../data/network.json"),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		RedisAddr:     os.Getenv("REDIS_ADDR"),
		MLURL:         os.Getenv("ML_URL"),
		MLTimeout:     time.Duration(mlTimeoutMs) * time.Millisecond,
		JWTSecret:     env("JWT_SECRET", "dev-only-change-me"),
		TicketSecret:  env("TICKET_SECRET", "dev-only-ticket-secret"),
		IngestKey:     env("INGEST_KEY", "dev-ingest-key"),
		AdminEmail:    env("ADMIN_EMAIL", "admin@metronav.local"),
		AdminPassword: env("ADMIN_PASSWORD", "admin12345"),
		CORSOrigins:   env("CORS_ORIGINS", "http://localhost:3000"),
		SimSpeed:      speed,
		TickInterval:  time.Duration(tickMs) * time.Millisecond,
		Timezone:      env("TZ_NAME", "Asia/Kolkata"),
		RateLimitRPS:  rps,
	}
}
