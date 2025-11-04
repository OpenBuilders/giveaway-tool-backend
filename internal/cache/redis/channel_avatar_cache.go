package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	rplatform "github.com/open-builders/giveaway-backend/internal/platform/redis"
)

// ChannelAvatarEntry stores resolved Telegram file_path for a channel avatar
// alongside the big_file_unique_id to detect avatar changes and fetched time.
type ChannelAvatarEntry struct {
	FilePath        string    `json:"file_path"`
	BigFileUniqueID string    `json:"big_file_unique_id"`
	FetchedAt       time.Time `json:"fetched_at"`
}

// ChannelAvatarCache provides Redis-based caching for channel avatar file paths.
type ChannelAvatarCache struct {
	client *rplatform.Client
	ttl    time.Duration
}

func NewChannelAvatarCache(client *rplatform.Client, ttl time.Duration) *ChannelAvatarCache {
	return &ChannelAvatarCache{client: client, ttl: ttl}
}

func (c *ChannelAvatarCache) key(chatID int64) string {
	return fmt.Sprintf("channel:%d:avatar", chatID)
}

// Get returns cached entry for a channel ID, or error when missing/failed.
func (c *ChannelAvatarCache) Get(ctx context.Context, chatID int64) (*ChannelAvatarEntry, error) {
	v, err := c.client.Get(ctx, c.key(chatID)).Bytes()
	if err != nil {
		return nil, err
	}
	var e ChannelAvatarEntry
	if err := json.Unmarshal(v, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

// Set stores the avatar entry with TTL.
func (c *ChannelAvatarCache) Set(ctx context.Context, chatID int64, e *ChannelAvatarEntry) error {
	if e.FetchedAt.IsZero() {
		e.FetchedAt = time.Now().UTC()
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, c.key(chatID), b, c.ttl).Err()
}

// Invalidate removes cached avatar for the channel ID.
func (c *ChannelAvatarCache) Invalidate(ctx context.Context, chatID int64) error {
	return c.client.Del(ctx, c.key(chatID)).Err()
}
