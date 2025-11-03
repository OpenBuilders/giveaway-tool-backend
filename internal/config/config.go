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
	// Public base URL for building links to this backend (e.g., for public avatars)
	PublicBaseURL string
	// DB migrations
	DBAutoMigrate bool
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
	// TON Lite client
	TonLiteConfigURL string // optional global config URL (defaults to https://ton.org/global-config.json)
	// WebApp
	WebAppBaseURL string // base URL for webapp, used in notifications buttons
}

// Load reads environment variables into Config with sane defaults for local dev.
func Load() (*Config, error) {
	cfg := &Config{
		HTTPAddr:           getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL:        getEnv("DATABASE_URL", "postgres://user:password@localhost:5432/giveaway?sslmode=disable"),
		RedisAddr:          getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:      getEnv("REDIS_PASSWORD", ""),
		PublicBaseURL:      getEnv("PUBLIC_BASE_URL", "https://dev-api.giveaway.tools.tg"),
		CORSAllowedOrigins: getEnv("CORS_ALLOWED_ORIGINS", "*"),
		TelegramBotToken:   getEnv("TELEGRAM_BOT_TOKEN", ""),
		TonProofDomain:     getEnv("TON_PROOF_DOMAIN", ""),
		TonAPIBaseURL:      getEnv("TONAPI_BASE_URL", "https://tonapi.io"),
		TonAPIToken:        getEnv("TONAPI_TOKEN", ""),
		TonLiteConfigURL:   getEnv("TON_LITE_CONFIG_URL", "https://ton.org/global-config.json"),
		WebAppBaseURL:      getEnv("WEBAPP_BASE_URL", ""),
	}
	redisDBStr := getEnv("REDIS_DB", "0")
	dbNum, err := strconv.Atoi(redisDBStr)
	if err != nil {
		return nil, fmt.Errorf("invalid REDIS_DB: %w", err)
	}
	if v := getEnv("TON_PROOF_PAYLOAD_TTL_SEC", "300"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.TonProofPayloadTTLSec = n
		} else {
			return nil, fmt.Errorf("invalid TON_PROOF_PAYLOAD_TTL_SEC: %w", err)
		}
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
	// DB_AUTO_MIGRATE: if true, app runs migrations on start
	if v := getEnv("DB_AUTO_MIGRATE", "false"); v != "" {
		cfg.DBAutoMigrate = v == "true" || v == "1" || v == "yes" || v == "on"
	}
	return cfg, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// GetPublicBaseURL returns the public base URL from environment with a sane default.
func GetPublicBaseURL() string {
	return getEnv("PUBLIC_BASE_URL", "http://localhost:8080")
}
