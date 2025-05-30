package config

import (
	"github.com/caarlos0/env/v10"
	"github.com/joho/godotenv"
)

type Config struct {
	Debug bool `env:"DEBUG" envDefault:"false"`

	Server struct {
		Port int `env:"PORT" envDefault:"8080"`
		Origin string `env:"ORIGIN" envDefault:"http://localhost:3000"`
	}

	Redis struct {
		Host     string `env:"REDIS_HOST" envDefault:"localhost"`
		Port     int    `env:"REDIS_PORT" envDefault:"6379"`
		Password string `env:"REDIS_PASSWORD" envDefault:""`
		DB       int    `env:"REDIS_DB" envDefault:"0"`
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
