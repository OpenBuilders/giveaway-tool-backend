package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"giveaway-tool-backend/internal/common/config"
	"giveaway-tool-backend/internal/common/logger"
	"time"

	_ "github.com/lib/pq"
)

type Client struct {
	db *sql.DB
}

func NewClient(cfg *config.Config) (*Client, error) {
	dsn := cfg.Postgres.GetDSN()

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Настройка пула соединений
	db.SetMaxOpenConns(cfg.Postgres.MaxOpenConns)
	db.SetMaxIdleConns(cfg.Postgres.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.Postgres.ConnMaxLifetime)

	// Проверяем соединение
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	logger.Info().
		Str("host", cfg.Postgres.Host).
		Int("port", cfg.Postgres.Port).
		Str("database", cfg.Postgres.Database).
		Msg("PostgreSQL client initialized")

	return &Client{db: db}, nil
}

// GetDB возвращает экземпляр базы данных
func (c *Client) GetDB() *sql.DB {
	return c.db
}

// Close закрывает соединение с базой данных
func (c *Client) Close() error {
	return c.db.Close()
}

// HealthCheck проверяет здоровье базы данных
func (c *Client) HealthCheck(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

// Stats возвращает статистику пула соединений
func (c *Client) Stats() sql.DBStats {
	return c.db.Stats()
}
