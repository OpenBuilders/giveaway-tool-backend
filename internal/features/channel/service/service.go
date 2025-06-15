package service

import (
	"context"
	"fmt"
	"giveaway-tool-backend/internal/features/channel/repository"
	channelredis "giveaway-tool-backend/internal/features/channel/repository/redis"
	"giveaway-tool-backend/internal/features/giveaway/models"
	"giveaway-tool-backend/internal/platform/telegram"
	"log"
	"strconv"

	"github.com/redis/go-redis/v9"
)

type ChannelService interface {
	GetChannelAvatar(ctx context.Context, channelID int64) (string, error)
	GetUserChannels(ctx context.Context, userID int64) ([]int64, error)
	GetChannelTitle(ctx context.Context, channelID int64) (string, error)
	GetChannelUsername(ctx context.Context, channelID int64) (string, error)
	GetPublicChannelInfo(ctx context.Context, username string) (*telegram.PublicChannelInfo, error)
	GetChannelInfoByID(ctx context.Context, channelID int64) (*models.ChannelInfo, error)
	GetChannelInfoByUsername(ctx context.Context, username string) (*models.ChannelInfo, error)
}

type channelService struct {
	repo           repository.ChannelRepository
	telegramClient *telegram.Client
	debug          bool
	redisClient    *redis.Client
}

func NewChannelService(repo repository.ChannelRepository, redisClient *redis.Client, debug bool) ChannelService {
	return &channelService{
		repo:           repo,
		telegramClient: telegram.NewClient(),
		debug:          debug,
		redisClient:    redisClient,
	}
}

// GetChannelAvatar получает CDN ссылку на аватар канала из Redis
func (s *channelService) GetChannelAvatar(ctx context.Context, channelID int64) (string, error) {
	if s.debug {
		log.Printf("[DEBUG] Getting channel avatar for channel %d", channelID)
	}

	key := fmt.Sprintf(channelredis.ChannelAvatarKey, channelID)
	avatarURL, err := s.redisClient.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return "", nil
		}
		return "", fmt.Errorf("failed to get channel avatar from redis: %w", err)
	}

	if s.debug {
		log.Printf("[DEBUG] Successfully retrieved avatar URL for channel %d", channelID)
	}

	return avatarURL, nil
}

// GetUserChannels получает список каналов пользователя из Redis
func (s *channelService) GetUserChannels(ctx context.Context, userID int64) ([]int64, error) {
	if s.debug {
		log.Printf("[DEBUG] Getting channels for user %d", userID)
	}

	key := fmt.Sprintf(channelredis.UserChannelsKey, userID)
	channels, err := s.redisClient.SMembers(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get user channels from redis: %w", err)
	}

	// Конвертируем строковые ID каналов в int64
	channelIDs := make([]int64, 0, len(channels))
	for _, channel := range channels {
		channelID, err := strconv.ParseInt(channel, 10, 64)
		if err != nil {
			if s.debug {
				log.Printf("[DEBUG] Failed to parse channel ID %s: %v", channel, err)
			}
			continue
		}
		channelIDs = append(channelIDs, channelID)
	}

	if s.debug {
		log.Printf("[DEBUG] Successfully retrieved %d channels for user %d", len(channelIDs), userID)
	}

	return channelIDs, nil
}

// GetChannelTitle получает название канала из Redis
func (s *channelService) GetChannelTitle(ctx context.Context, channelID int64) (string, error) {
	if s.debug {
		log.Printf("[DEBUG] Getting title for channel %d", channelID)
	}

	key := fmt.Sprintf(channelredis.ChannelTitleKey, channelID)
	title, err := s.redisClient.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return "", nil
		}
		return "", fmt.Errorf("failed to get channel title from redis: %w", err)
	}

	if s.debug {
		log.Printf("[DEBUG] Successfully retrieved title for channel %d", channelID)
	}

	return title, nil
}

// GetChannelUsername получает username канала из Redis
func (s *channelService) GetChannelUsername(ctx context.Context, channelID int64) (string, error) {
	return s.repo.GetChannelUsername(ctx, channelID)
}

// GetPublicChannelInfo получает публичную информацию о канале
func (s *channelService) GetPublicChannelInfo(ctx context.Context, username string) (*telegram.PublicChannelInfo, error) {
	pubInfo, err := s.telegramClient.GetPublicChannelInfo(ctx, username, s.repo)
	if err != nil {
		return nil, err
	}

	// Сохраняем всю структуру ChannelInfo в Redis
	info := models.ChannelInfo{
		ID:         pubInfo.ID,
		Username:   pubInfo.Username,
		Title:      pubInfo.Title,
		AvatarURL:  pubInfo.AvatarURL,
		ChannelURL: pubInfo.ChannelURL,
	}
	_ = s.repo.SetChannelInfo(ctx, info)

	return pubInfo, nil
}

func (s *channelService) GetChannelInfoByID(ctx context.Context, channelID int64) (*models.ChannelInfo, error) {
	return s.repo.GetChannelInfoByID(ctx, channelID)
}

func (s *channelService) GetChannelInfoByUsername(ctx context.Context, username string) (*models.ChannelInfo, error) {
	return s.repo.GetChannelInfoByUsername(ctx, username)
}
