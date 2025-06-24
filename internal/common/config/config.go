package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Server   ServerConfig
	Redis    RedisConfig
	Postgres PostgresConfig
	Cache    CacheConfig
	Debug    bool
}

type ServerConfig struct {
	Port   int
	Origin string
}

type RedisConfig struct {
	Host     string
	Port     int
	Password string
	DB       int
	PoolSize int

	// Конфигурация для шардирования
	EnableSharding bool

	// Шарды для записи (через запятую: host:port:password:db)
	WriteShards []string

	// Шарды для чтения (через запятую: host:port:password:db)
	ReadShards []string

	// Стратегия распределения ключей
	ShardingStrategy string
}

type PostgresConfig struct {
	Host            string
	Port            int
	User            string
	Password        string
	Database        string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type CacheConfig struct {
	// TTL for different types of cached data
	GiveawayTTL             time.Duration
	UserTTL                 time.Duration
	ChannelTTL              time.Duration
	UserStatsTTL            time.Duration
	UserGiveawaysTTL        time.Duration
	TopGiveawaysTTL         time.Duration
	PrizeTemplatesTTL       time.Duration
	RequirementTemplatesTTL time.Duration
}

func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Port:   getEnvAsInt("SERVER_PORT", 8080),
			Origin: getEnv("SERVER_ORIGIN", "http://localhost:3000"),
		},
		Redis: RedisConfig{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnvAsInt("REDIS_PORT", 6379),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvAsInt("REDIS_DB", 0),
			PoolSize: getEnvAsInt("REDIS_POOL_SIZE", 10),
		},
		Postgres: PostgresConfig{
			Host:            getEnv("POSTGRES_HOST", "localhost"),
			Port:            getEnvAsInt("POSTGRES_PORT", 5432),
			User:            getEnv("POSTGRES_USER", "postgres"),
			Password:        getEnv("POSTGRES_PASSWORD", ""),
			Database:        getEnv("POSTGRES_DB", "giveaway_tool"),
			SSLMode:         getEnv("POSTGRES_SSLMODE", "disable"),
			MaxOpenConns:    getEnvAsInt("POSTGRES_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getEnvAsInt("POSTGRES_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: getEnvAsDuration("POSTGRES_CONN_MAX_LIFETIME", 5*time.Minute),
		},
		Cache: CacheConfig{
			GiveawayTTL:             getEnvAsDuration("CACHE_GIVEAWAY_TTL", 5*time.Minute),
			UserTTL:                 getEnvAsDuration("CACHE_USER_TTL", 5*time.Minute),
			ChannelTTL:              getEnvAsDuration("CACHE_CHANNEL_TTL", 5*time.Minute),
			UserStatsTTL:            getEnvAsDuration("CACHE_USER_STATS_TTL", 5*time.Minute),
			UserGiveawaysTTL:        getEnvAsDuration("CACHE_USER_GIVEAWAYS_TTL", 5*time.Minute),
			TopGiveawaysTTL:         getEnvAsDuration("CACHE_TOP_GIVEAWAYS_TTL", 5*time.Minute),
			PrizeTemplatesTTL:       getEnvAsDuration("CACHE_PRIZE_TEMPLATES_TTL", 5*time.Minute),
			RequirementTemplatesTTL: getEnvAsDuration("CACHE_REQUIREMENT_TEMPLATES_TTL", 5*time.Minute),
		},
		Debug: getEnvAsBool("DEBUG", false),
	}
}

func (c *PostgresConfig) GetDSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Database, c.SSLMode)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvAsBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func getEnvAsDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
