package models

import "time"

// Giveaway представляет гив для пользовательского модуля
type Giveaway struct {
	ID              string    `json:"id"`
	CreatorID       int64     `json:"creator_id"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	StartedAt       time.Time `json:"started_at"`
	EndsAt          time.Time `json:"ends_at"`
	Duration        int64     `json:"duration"`
	MaxParticipants int       `json:"max_participants,omitempty"`
	WinnersCount    int       `json:"winners_count"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	AutoDistribute  bool      `json:"auto_distribute,omitempty"`
	AllowTickets    bool      `json:"allow_tickets"`
	MsgID           int64     `json:"msg_id"`
}

// WinRecord представляет запись о победе
type WinRecord struct {
	ID         string     `json:"id"`
	GiveawayID string     `json:"giveaway_id"`
	UserID     int64      `json:"user_id"`
	PrizeID    string     `json:"prize_id"`
	Place      int        `json:"place"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	ReceivedAt *time.Time `json:"received_at,omitempty"`
}
