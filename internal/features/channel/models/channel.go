package models

import "time"

// Константы статусов канала
const (
	ChannelStatusActive   = "active"
	ChannelStatusInactive = "inactive"
	ChannelStatusBanned   = "banned"
)

// Channel представляет полную модель канала в системе
type Channel struct {
	ID        int64     `json:"id" example:"-1001234567890" description:"ID канала в Telegram"`
	Username  string    `json:"username" example:"mychannel" description:"Username канала"`
	Title     string    `json:"title" example:"My Channel" description:"Название канала"`
	AvatarURL string    `json:"avatar_url" example:"https://t.me/i/userpic/320/username.jpg" description:"URL аватара канала"`
	Status    string    `json:"status" example:"active" enums:"active,inactive,banned" description:"Статус канала"`
	CreatedAt time.Time `json:"created_at" example:"2024-03-15T14:30:00Z" description:"Дата создания"`
	UpdatedAt time.Time `json:"updated_at" example:"2024-03-15T14:30:00Z" description:"Дата последнего обновления"`
}

// ChannelInfo представляет информацию о канале
type ChannelInfo struct {
	ID         int64     `json:"id"`
	Username   string    `json:"username"`
	Title      string    `json:"title"`
	AvatarURL  string    `json:"avatar_url"`
	ChannelURL string    `json:"channel_url"`
	CreatedAt  time.Time `json:"created_at"`
}

// ChannelStats представляет статистику канала
type ChannelStats struct {
	ChannelID         int64 `json:"channel_id"`
	TotalGiveaways    int   `json:"total_giveaways"`
	ActiveGiveaways   int   `json:"active_giveaways"`
	TotalParticipants int   `json:"total_participants"`
	TotalWinners      int   `json:"total_winners"`
}
