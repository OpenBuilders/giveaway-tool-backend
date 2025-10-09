package giveaway

import "time"

// GiveawayStatus represents the lifecycle state of a giveaway.
type GiveawayStatus string

const (
	GiveawayStatusScheduled GiveawayStatus = "scheduled"
	GiveawayStatusActive    GiveawayStatus = "active"
	GiveawayStatusFinished  GiveawayStatus = "finished"
	GiveawayStatusCancelled GiveawayStatus = "cancelled"
)

// PrizePlace describes a prize for a specific winning place.
type PrizePlace struct {
	// Place is optional: when nil, the prize is unassigned and should be
	// randomly distributed among winners.
	Place       *int   `json:"place,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	// Quantity applies only to unassigned prizes; defaults to 1 for place-bound.
	Quantity int `json:"quantity,omitempty"`
}

// ChannelInfo describes a sponsor Telegram channel or user.
type ChannelInfo struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	URL      string `json:"url,omitempty"`
	Title    string `json:"title,omitempty"`
}

// Giveaway is the aggregate representing a giveaway created by a user.
type Giveaway struct {
	ID                string         `json:"id"`
	CreatorID         int64          `json:"creator_id"`
	Title             string         `json:"title"`
	Description       string         `json:"description"`
	StartedAt         time.Time      `json:"started_at"`
	EndsAt            time.Time      `json:"ends_at"`
	Duration          int64          `json:"duration"`
	MaxWinnersCount   int            `json:"winners_count"`
	Status            GiveawayStatus `json:"status"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	Prizes            []PrizePlace   `json:"prizes,omitempty"`
	Sponsors          []ChannelInfo  `json:"sponsors"`
	Requirements      []Requirement  `json:"requirements,omitempty"`
	Winners           []Winner       `json:"winners,omitempty"`
	ParticipantsCount int            `json:"participants_count"`
}

// WinnerPrize describes a prize assigned to a winner.
type WinnerPrize struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// Winner represents a winner with place and assigned prizes.
type Winner struct {
	Place  int           `json:"place"`
	UserID int64         `json:"user_id"`
	Prizes []WinnerPrize `json:"prizes,omitempty"`
}
