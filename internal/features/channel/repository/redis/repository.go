package redis

import (
	"context"
	"fmt"
	"giveaway-tool-backend/internal/features/channel/repository"

	"github.com/redis/go-redis/v9"
)

const (
	ChannelUsernameKey = "channel:%d:username"
	ChannelAvatarKey   = "channel:%s:avatar"
	ChannelTitleKey    = "channel:%d:title"
	UserChannelsKey    = "user:%d:channels"
)

type redisRepository struct {
	client *redis.Client
}

func NewRedisChannelRepository(client *redis.Client) repository.ChannelRepository {
	return &redisRepository{
		client: client,
	}
}

func (r *redisRepository) GetChannelUsername(ctx context.Context, channelID int64) (string, error) {
	key := fmt.Sprintf(ChannelUsernameKey, channelID)
	return r.client.Get(ctx, key).Result()
}

func (r *redisRepository) SetChannelAvatar(ctx context.Context, username string, avatarURL string) error {
	key := fmt.Sprintf(ChannelAvatarKey, username)
	return r.client.Set(ctx, key, avatarURL, 0).Err()
}
