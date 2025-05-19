package models

import "time"

// PrizeType представляет тип приза
type PrizeType string

const (
	// Internal prizes (автоматическая выдача)
	PrizeTypeTelegramStars   PrizeType = "telegram_stars"
	PrizeTypeTelegramPremium PrizeType = "telegram_premium"
	PrizeTypeTelegramGifts   PrizeType = "telegram_gifts"

	// External prizes (ручная выдача)
	PrizeTypeTelegramStickers PrizeType = "telegram_stickers"
	PrizeTypeCustom           PrizeType = "custom"
	PrizeTypeInternal         PrizeType = "internal"
)

// PrizeStatus представляет статус приза
type PrizeStatus string

const (
	PrizeStatusPending     PrizeStatus = "pending"     // Ожидает выдачи
	PrizeStatusDistributed PrizeStatus = "distributed" // Выдан
	PrizeStatusCancelled   PrizeStatus = "cancelled"   // Отменен
)

// Prize представляет приз
type Prize struct {
	ID          string    `json:"id"`
	Type        PrizeType `json:"type"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsInternal  bool      `json:"is_internal"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// PrizePlace представляет приз для определенного места
type PrizePlace struct {
	Place     int    `json:"place" binding:"required,min=1"`
	PrizeID   string `json:"prize_id"`
	PrizeType string `json:"prize_type" binding:"required"`
}

// WinRecord представляет запись о выигрыше
type WinRecord struct {
	ID         string      `json:"id"`
	GiveawayID string      `json:"giveaway_id"`
	UserID     int64       `json:"user_id"`
	PrizeID    string      `json:"prize_id"`
	Place      int         `json:"place"`
	Status     PrizeStatus `json:"status"`
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
	ReceivedAt *time.Time  `json:"received_at,omitempty"`
}

// CustomPrizeCreate представляет данные для создания пользовательского приза
type CustomPrizeCreate struct {
	Name        string `json:"name" binding:"required,min=3,max=100"`
	Description string `json:"description" binding:"required,min=10,max=1000"`
}

// PrizeTemplate представляет шаблон приза
type PrizeTemplate struct {
	ID          string    `json:"id"`
	Type        PrizeType `json:"type"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsInternal  bool      `json:"is_internal"`
}
