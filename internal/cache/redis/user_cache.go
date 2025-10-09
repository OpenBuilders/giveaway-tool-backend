package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	rplatform "github.com/your-org/giveaway-backend/internal/platform/redis"
	domain "github.com/your-org/giveaway-backend/internal/domain/user"
)

// UserCache provides Redis-based caching for users.
type UserCache struct {
	client *rplatform.Client
	ttl    time.Duration
}

func NewUserCache(client *rplatform.Client, ttl time.Duration) *UserCache {
	return &UserCache{client: client, ttl: ttl}
}

func (c *UserCache) keyByID(id int64) string { return fmt.Sprintf("user:id:%d", id) }
func (c *UserCache) keyByUsername(username string) string { return fmt.Sprintf("user:username:%s", username) }

// Set stores user by id and username keys.
func (c *UserCache) Set(ctx context.Context, u *domain.User) error {
	b, err := json.Marshal(u)
	if err != nil {
		return err
	}
	idKey := c.keyByID(u.ID)
	if err := c.client.Set(ctx, idKey, b, c.ttl).Err(); err != nil {
		return err
	}
	if u.Username != "" {
		if err := c.client.Set(ctx, c.keyByUsername(u.Username), b, c.ttl).Err(); err != nil {
			return err
		}
	}
	return nil
}

// GetByID returns cached user by id.
func (c *UserCache) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	v, err := c.client.Get(ctx, c.keyByID(id)).Bytes()
	if err != nil {
		return nil, err
	}
	var u domain.User
	if err := json.Unmarshal(v, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// GetByUsername returns cached user by username.
func (c *UserCache) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	v, err := c.client.Get(ctx, c.keyByUsername(username)).Bytes()
	if err != nil {
		return nil, err
	}
	var u domain.User
	if err := json.Unmarshal(v, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// Invalidate removes cached entries for the user.
func (c *UserCache) Invalidate(ctx context.Context, u *domain.User) error {
	if err := c.client.Del(ctx, c.keyByID(u.ID)).Err(); err != nil {
		return err
	}
	if u.Username != "" {
		if err := c.client.Del(ctx, c.keyByUsername(u.Username)).Err(); err != nil {
			return err
		}
	}
	return nil
}
