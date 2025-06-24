package service

import (
	"context"
	"giveaway-tool-backend/internal/features/channel/models"
	"giveaway-tool-backend/internal/platform/telegram"
)

type ChannelService interface {
	// Методы для работы с ChannelInfo
	GetChannelAvatar(ctx context.Context, channelID int64) (string, error)
	GetUserChannels(ctx context.Context, userID int64) ([]int64, error)
	GetChannelTitle(ctx context.Context, channelID int64) (string, error)
	GetChannelUsername(ctx context.Context, channelID int64) (string, error)
	GetPublicChannelInfo(ctx context.Context, username string) (*telegram.PublicChannelInfo, error)
	GetChannelInfoByID(ctx context.Context, channelID int64) (*models.ChannelInfo, error)
	GetChannelInfoByUsername(ctx context.Context, username string) (*models.ChannelInfo, error)

	// Методы для работы с полной моделью Channel
	CreateChannel(ctx context.Context, channel *models.Channel) error
	GetChannel(ctx context.Context, id int64) (*models.Channel, error)
	UpdateChannel(ctx context.Context, channel *models.Channel) error
	DeleteChannel(ctx context.Context, id int64) error
	BanChannel(ctx context.Context, id int64, reason string) error
	UnbanChannel(ctx context.Context, id int64) error
	ListChannels(ctx context.Context, offset, limit int) ([]*models.Channel, error)
	GetChannelStats(ctx context.Context, channelID int64) (*models.ChannelStats, error)
}
