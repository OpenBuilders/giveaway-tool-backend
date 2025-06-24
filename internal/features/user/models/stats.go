package models

// UserStats представляет статистику пользователя
type UserStats struct {
	CreatedGiveaways  int `json:"created_giveaways"`
	ParticipatedCount int `json:"participated_count"`
	WinsCount         int `json:"wins_count"`
}
