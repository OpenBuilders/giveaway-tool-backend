package config

import (
	"github.com/caarlos0/env/v10"
	"github.com/joho/godotenv"
)

type Config struct {
	Debug bool `env:"DEBUG" envDefault:"false"`

	Server struct {
		Port   int    `env:"PORT" envDefault:"8080"`
		Origin string `env:"ORIGIN" envDefault:"http://localhost:3000"`
	}

	Redis struct {
		Host     string `env:"REDIS_HOST" envDefault:"localhost"`
		Port     int    `env:"REDIS_PORT" envDefault:"6379"`
		Password string `env:"REDIS_PASSWORD" envDefault:""`
		DB       int    `env:"REDIS_DB" envDefault:"0"`

		// Конфигурация для шардирования
		EnableSharding bool `env:"REDIS_ENABLE_SHARDING" envDefault:"false"`

		// Шарды для записи (через запятую: host:port:password:db)
		WriteShards []string `env:"REDIS_WRITE_SHARDS" envSeparator:","`

		// Шарды для чтения (через запятую: host:port:password:db)
		ReadShards []string `env:"REDIS_READ_SHARDS" envSeparator:","`

		// Стратегия распределения ключей
		ShardingStrategy string `env:"REDIS_SHARDING_STRATEGY" envDefault:"hash"` // hash, round_robin, consistent_hash
	}

	Telegram struct {
		BotToken string   `env:"BOT_TOKEN,required"`
		Debug    bool     `env:"TELEGRAM_DEBUG" envDefault:"false"`
		AdminIDs []string `env:"ADMIN_IDS" envSeparator:","`
	}
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		// Игнорируем ошибку, если .env файл не найден
		// В production окружении переменные могут быть установлены напрямую
	}

	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		panic(err)
	}

	return cfg
}
