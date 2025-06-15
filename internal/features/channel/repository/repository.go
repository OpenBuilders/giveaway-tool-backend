package repository

import "context"

type ChannelRepository interface {
	GetChannelUsername(ctx context.Context, channelID int64) (string, error)
	SetChannelAvatar(ctx context.Context, username string, avatarURL string) error
}
