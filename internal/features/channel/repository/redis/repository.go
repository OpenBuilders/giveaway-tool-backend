package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"giveaway-tool-backend/internal/features/channel/repository"
	"giveaway-tool-backend/internal/features/giveaway/models"

	"github.com/redis/go-redis/v9"
)

const (
	ChannelUsernameKey       = "channel:%d:username"
	ChannelAvatarKey         = "channel:%s:avatar"
	ChannelTitleKey          = "channel:%d:title"
	UserChannelsKey          = "user:%d:channels"
	ChannelInfoByIDKey       = "channel:info:id:%d"
	ChannelInfoByUsernameKey = "channel:info:username:%s"
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
	if err := r.client.Set(ctx, fmt.Sprintf(ChannelInfoByIDKey, info.ID), data, 0).Err(); err != nil {
		return err
	}
	if info.Username != "" {
		if err := r.client.Set(ctx, fmt.Sprintf(ChannelInfoByUsernameKey, info.Username), data, 0).Err(); err != nil {
			return err
		}
	}
	return nil
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
