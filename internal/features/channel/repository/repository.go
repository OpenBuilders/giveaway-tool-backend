package repository

import (
	"context"
	"giveaway-tool-backend/internal/features/giveaway/models"
)

type ChannelRepository interface {
	GetChannelUsername(ctx context.Context, channelID int64) (string, error)
	SetChannelAvatar(ctx context.Context, username string, avatarURL string) error
	GetChannelTitle(ctx context.Context, channelID int64) (string, error)

	// Новый метод для полной информации
	SetChannelInfo(ctx context.Context, info models.ChannelInfo) error
	GetChannelInfoByID(ctx context.Context, channelID int64) (*models.ChannelInfo, error)
	GetChannelInfoByUsername(ctx context.Context, username string) (*models.ChannelInfo, error)
}
