package models

import "time"

type PrizeType string

const (
	PrizeTypeTelegramStars   PrizeType = "telegram_stars"
	PrizeTypeTelegramPremium PrizeType = "telegram_premium"
	PrizeTypeTelegramGifts   PrizeType = "telegram_gifts"

	PrizeTypeTelegramStickers PrizeType = "telegram_stickers"
	PrizeTypeCustom           PrizeType = "custom"
	PrizeTypeInternal         PrizeType = "internal"
)

type PrizeStatus string

const (
	PrizeStatusPending     PrizeStatus = "pending"     // Ожидает выдачи
	PrizeStatusDistributed PrizeStatus = "distributed" // Выдан
	PrizeStatusCancelled   PrizeStatus = "cancelled"   // Отменен
)

type Prize struct {
	ID          string             `json:"id"`
	Type        PrizeType          `json:"type"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	IsInternal  bool               `json:"is_internal"`
	Fields      []CustomPrizeField `json:"fields,omitempty"` // Only for custom prizes
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
}

type PrizePlace struct {
	Place     int                `json:"place" binding:"required,min=1"`
	PrizeID   string             `json:"prize_id"`
	PrizeType PrizeType          `json:"prize_type" binding:"required"`
	Fields    []CustomPrizeField `json:"fields,omitempty"`
}

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

type CustomPrizeCreate struct {
	Name        string `json:"name" binding:"required,min=3,max=100"`
	Description string `json:"description" binding:"required,min=10,max=1000"`
}

type PrizeTemplate struct {
	ID          string    `json:"id"`
	Type        PrizeType `json:"type"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsInternal  bool      `json:"is_internal"`
}

func IsPrizeInternal(prizeType PrizeType) bool {
	switch prizeType {
	case PrizeTypeTelegramStars, PrizeTypeTelegramPremium, PrizeTypeTelegramGifts, PrizeTypeInternal:
		return true
	default:
		return false
	}
}
