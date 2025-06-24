package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"giveaway-tool-backend/internal/platform/redis"
)

type CacheService struct {
	redisClient redis.RedisClient
}

func NewCacheService(redisClient redis.RedisClient) *CacheService {
	return &CacheService{
		redisClient: redisClient,
	}
}

// Get получает значение из кэша
func (c *CacheService) Get(ctx context.Context, key string, dest interface{}) error {
	data, err := c.redisClient.Get(ctx, key).Result()
	if err != nil {
		return err
	}

	return json.Unmarshal([]byte(data), dest)
}

// Set сохраняет значение в кэш
func (c *CacheService) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	return c.redisClient.Set(ctx, key, string(data), ttl).Err()
}

// Delete удаляет значение из кэша
func (c *CacheService) Delete(ctx context.Context, key string) error {
	return c.redisClient.Del(ctx, key).Err()
}

// DeletePattern удаляет все ключи по паттерну
func (c *CacheService) DeletePattern(ctx context.Context, pattern string) error {
	keys, err := c.redisClient.Keys(ctx, pattern).Result()
	if err != nil {
		return err
	}

	if len(keys) > 0 {
		return c.redisClient.Del(ctx, keys...).Err()
	}

	return nil
}

// Exists проверяет существование ключа
func (c *CacheService) Exists(ctx context.Context, key string) (bool, error) {
	result, err := c.redisClient.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return result > 0, nil
}

// GetOrSet получает значение из кэша или устанавливает новое
func (c *CacheService) GetOrSet(ctx context.Context, key string, dest interface{}, ttl time.Duration, setter func() (interface{}, error)) error {
	// Пытаемся получить из кэша
	err := c.Get(ctx, key, dest)
	if err == nil {
		return nil
	}

	// Если не найдено, вызываем setter
	value, err := setter()
	if err != nil {
		return err
	}

	// Сохраняем в кэш
	err = c.Set(ctx, key, value, ttl)
	if err != nil {
		return err
	}

	// Копируем значение в dest
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, dest)
}

// InvalidateUserCache инвалидирует кэш пользователя
func (c *CacheService) InvalidateUserCache(ctx context.Context, userID int64) error {
	patterns := []string{
		fmt.Sprintf("user:%d", userID),
		fmt.Sprintf("user_stats:%d", userID),
		fmt.Sprintf("user_giveaways:%d:*", userID),
		fmt.Sprintf("user_wins:%d", userID),
	}

	for _, pattern := range patterns {
		if err := c.DeletePattern(ctx, pattern); err != nil {
			return fmt.Errorf("failed to delete pattern %s: %w", pattern, err)
		}
	}

	return nil
}

// InvalidateGiveawayCache инвалидирует кэш гива
func (c *CacheService) InvalidateGiveawayCache(ctx context.Context, giveawayID string) error {
	patterns := []string{
		fmt.Sprintf("giveaway:%s", giveawayID),
		fmt.Sprintf("giveaway_participants:%s", giveawayID),
		fmt.Sprintf("giveaway_winners:%s", giveawayID),
		fmt.Sprintf("giveaway_prizes:%s", giveawayID),
		"active_giveaways",
		"pending_giveaways",
		"top_giveaways",
	}

	for _, pattern := range patterns {
		if err := c.DeletePattern(ctx, pattern); err != nil {
			return fmt.Errorf("failed to delete pattern %s: %w", pattern, err)
		}
	}

	return nil
}

// InvalidateChannelCache инвалидирует кэш канала
func (c *CacheService) InvalidateChannelCache(ctx context.Context, channelID int64) error {
	patterns := []string{
		fmt.Sprintf("channel:%d", channelID),
		fmt.Sprintf("channel_avatar:%d", channelID),
	}

	for _, pattern := range patterns {
		if err := c.DeletePattern(ctx, pattern); err != nil {
			return fmt.Errorf("failed to delete pattern %s: %w", pattern, err)
		}
	}

	return nil
}
