package repository

import (
	"context"
	"errors"
	"giveaway-tool-backend/internal/features/channel/models"
)

var (
	ErrChannelNotFound = errors.New("channel not found")
)

type ChannelRepository interface {
	// Методы для работы с ChannelInfo
	GetChannelUsername(ctx context.Context, channelID int64) (string, error)
	SetChannelAvatar(ctx context.Context, username string, avatarURL string) error
	GetChannelTitle(ctx context.Context, channelID int64) (string, error)
	SaveChannelInfo(ctx context.Context, info *models.ChannelInfo) error
	GetChannelInfo(ctx context.Context, channelID int64) (*models.ChannelInfo, error)
	GetChannelInfoByID(ctx context.Context, channelID int64) (*models.ChannelInfo, error)
	GetChannelInfoByUsername(ctx context.Context, username string) (*models.ChannelInfo, error)
	GetChannelsByIDs(ctx context.Context, ids []int64) ([]*models.ChannelInfo, error)

	// Методы для работы с полной моделью Channel
	Create(ctx context.Context, channel *models.Channel) error
	GetByID(ctx context.Context, id int64) (*models.Channel, error)
	Update(ctx context.Context, channel *models.Channel) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, offset, limit int) ([]*models.Channel, error)
	GetChannelStats(ctx context.Context, channelID int64) (*models.ChannelStats, error)
}
