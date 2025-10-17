package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds application configuration loaded from environment variables.
type Config struct {
	HTTPAddr      string
	DatabaseURL   string
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	// CORS settings
	CORSAllowedOrigins string
	// Telegram init-data validation settings
	TelegramBotToken string // Bot token for first-party validation
	InitDataTTL      int    // TTL in seconds for init-data expiration (0 to skip)
	// Workers
	GiveawayExpireIntervalSec int // background worker tick seconds
	// TON Proof
	TonProofDomain        string // expected domain in proof
	TonProofPayloadTTLSec int    // TTL for payloads
	TonAPIBaseURL         string // optional TonAPI base URL
	TonAPIToken           string // optional TonAPI token (Bearer)
}

// Load reads environment variables into Config with sane defaults for local dev.
func Load() (*Config, error) {
	cfg := &Config{
		HTTPAddr:           getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL:        getEnv("DATABASE_URL", "postgres://user:password@localhost:5432/giveaway?sslmode=disable"),
		RedisAddr:          getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:      getEnv("REDIS_PASSWORD", ""),
		CORSAllowedOrigins: getEnv("CORS_ALLOWED_ORIGINS", "*"),
		TelegramBotToken:   getEnv("TELEGRAM_BOT_TOKEN", ""),
		TonProofDomain:     getEnv("TON_PROOF_DOMAIN", ""),
		TonAPIBaseURL:      getEnv("TONAPI_BASE_URL", "https://tonapi.io"),
		TonAPIToken:        getEnv("TONAPI_TOKEN", ""),
	}
	redisDBStr := getEnv("REDIS_DB", "0")
	dbNum, err := strconv.Atoi(redisDBStr)
	if err != nil {
		return nil, fmt.Errorf("invalid REDIS_DB: %w", err)
	}
	cfg.RedisDB = dbNum
	if ttlStr := getEnv("INIT_DATA_TTL", "86400"); ttlStr != "" { // default 24h
		if ttl, err := strconv.Atoi(ttlStr); err == nil {
			cfg.InitDataTTL = ttl
		} else {
			return nil, fmt.Errorf("invalid INIT_DATA_TTL: %w", err)
		}
	}
	if iv := getEnv("GIVEAWAY_EXPIRE_INTERVAL_SEC", "30"); iv != "" {
		if n, err := strconv.Atoi(iv); err == nil {
			cfg.GiveawayExpireIntervalSec = n
		} else {
			return nil, fmt.Errorf("invalid GIVEAWAY_EXPIRE_INTERVAL_SEC: %w", err)
		}
	}
	return cfg, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
