package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// Client wraps go-redis client to allow future extensions.
type Client struct {
	*redis.Client
}

// Open creates a new Redis client and pings it to validate the connection.
func Open(ctx context.Context, addr, password string, db int) (*Client, error) {
	if addr == "" {
		return nil, fmt.Errorf("empty redis addr")
	}
	c := redis.NewClient(&redis.Options{Addr: addr, Password: password, DB: db})
	if err := c.Ping(ctx).Err(); err != nil {
		_ = c.Close()
		return nil, err
	}
	return &Client{Client: c}, nil
}
