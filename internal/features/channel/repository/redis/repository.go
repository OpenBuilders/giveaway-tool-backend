package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"giveaway-tool-backend/internal/features/channel/repository"
	"giveaway-tool-backend/internal/features/giveaway/models"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	ChannelUsernameKey       = "channel:%d:username"
	ChannelAvatarKey         = "channel:%s:avatar"
	ChannelTitleKey          = "channel:%d:title"
	UserChannelsKey          = "user:%d:channels"
	ChannelInfoByIDKey       = "channel:info:id:%d"
	ChannelInfoByUsernameKey = "channel:info:username:%s"
	ChannelInfoTTL           = 14 * 24 * time.Hour // 14 дней
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

func (r *redisRepository) GetChannelTitle(ctx context.Context, channelID int64) (string, error) {
	key := fmt.Sprintf(ChannelTitleKey, channelID)
	return r.client.Get(ctx, key).Result()
}

func (r *redisRepository) SetChannelInfo(ctx context.Context, info models.ChannelInfo) error {
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}

	pipe := r.client.Pipeline()
	pipe.Set(ctx, fmt.Sprintf(ChannelInfoByIDKey, info.ID), data, ChannelInfoTTL)
	if info.Username != "" {
		pipe.Set(ctx, fmt.Sprintf(ChannelInfoByUsernameKey, info.Username), data, ChannelInfoTTL)
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (r *redisRepository) GetChannelInfoByID(ctx context.Context, channelID int64) (*models.ChannelInfo, error) {
	data, err := r.client.Get(ctx, fmt.Sprintf(ChannelInfoByIDKey, channelID)).Result()
	if err != nil {
		return nil, err
	}
	var info models.ChannelInfo
	if err := json.Unmarshal([]byte(data), &info); err != nil {
		return nil, err
	}
	return &info, nil
}

func (r *redisRepository) GetChannelInfoByUsername(ctx context.Context, username string) (*models.ChannelInfo, error) {
	data, err := r.client.Get(ctx, fmt.Sprintf(ChannelInfoByUsernameKey, username)).Result()
	if err != nil {
		return nil, err
	}
	var info models.ChannelInfo
	if err := json.Unmarshal([]byte(data), &info); err != nil {
		return nil, err
	}
	return &info, nil
}
