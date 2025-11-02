package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	rplatform "github.com/open-builders/giveaway-backend/internal/platform/redis"
)

// ChannelPhotoEntry stores minimal Telegram photo identifiers for a chat
// used to detect avatar changes without calling getChat each time.
type ChannelPhotoEntry struct {
	ID              int64     `json:"id"`
	BigFileID       string    `json:"big_file_id"`
	BigFileUniqueID string    `json:"big_file_unique_id"`
	FetchedAt       time.Time `json:"fetched_at"`
}

// ChannelPhotoCache provides Redis-based short-lived caching for chat photo ids.
type ChannelPhotoCache struct {
	client *rplatform.Client
	ttl    time.Duration
}

func NewChannelPhotoCache(client *rplatform.Client, ttl time.Duration) *ChannelPhotoCache {
	return &ChannelPhotoCache{client: client, ttl: ttl}
}

func (c *ChannelPhotoCache) key(chatRef string) string {
	return fmt.Sprintf("channel:%s:photo", chatRef)
}

// Get returns cached photo entry for a chat reference (username or id), or error when missing/failed.
func (c *ChannelPhotoCache) Get(ctx context.Context, chatRef string) (*ChannelPhotoEntry, error) {
	v, err := c.client.Get(ctx, c.key(chatRef)).Bytes()
	if err != nil {
		return nil, err
	}
	var e ChannelPhotoEntry
	if err := json.Unmarshal(v, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

// Set stores the photo entry with TTL.
func (c *ChannelPhotoCache) Set(ctx context.Context, chatRef string, e *ChannelPhotoEntry) error {
	if e.FetchedAt.IsZero() {
		e.FetchedAt = time.Now().UTC()
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, c.key(chatRef), b, c.ttl).Err()
}

// Invalidate removes cached entry for the chat reference.
func (c *ChannelPhotoCache) Invalidate(ctx context.Context, chatRef string) error {
	return c.client.Del(ctx, c.key(chatRef)).Err()
}
