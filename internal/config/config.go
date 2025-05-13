package config

import (
	"log"
	"time"

	"github.com/caarlos0/env/v10"
	"github.com/joho/godotenv"
)

type Config struct {
	// HTTP
	Port string `env:"PORT" envDefault:"8080"`

	// Redis
	RedisHost     string        `env:"REDIS_HOST"     envDefault:"localhost:6379"`
	RedisPassword string        `env:"REDIS_PASSWORD" envDefault:""`
	RedisDB       int           `env:"REDIS_DB"       envDefault:"0"`
	RedisTTL      time.Duration `env:"REDIS_TTL"      envDefault:"5m"`

	// Misc
	Debug bool `env:"DEBUG" envDefault:"false"`
}

func MustLoad() *Config {
	_ = godotenv.Load()

	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		log.Fatalf("config: %v", err)
	}
	return cfg
}
