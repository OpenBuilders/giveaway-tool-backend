package dto

import (
	"github.com/go-playground/validator/v10"
	"giveaway-tool-backend/internal/features/giveaway/models"
	"time"
)

// GiveawayCreateRequest represents the request body for creating a new giveaway
type GiveawayCreateRequest struct {
	Title           string                `json:"title" binding:"required,min=3,max=100"`
	Description     string                `json:"description" binding:"max=1000"`
	Duration        int64                 `json:"duration" binding:"required,min=5"` // in seconds
	MaxParticipants int                   `json:"max_participants" binding:"min=0"`  // 0 = unlimited
	WinnersCount    int                   `json:"winners_count" binding:"required,min=1"`
	Prizes          []models.PrizePlace   `json:"prizes" binding:"required,dive"`
	AutoDistribute  bool                  `json:"auto_distribute"`
	AllowTickets    bool                  `json:"allow_tickets"`
	Requirements    []models.Requirements `json:"requirements" binding:"dive"`
	Validate        validator.FieldLevel
}

// GiveawayResponse represents the response body for giveaway operations
type GiveawayResponse struct {
	ID                string                `json:"id"`
	CreatorID         int64                 `json:"creator_id"`
	Title             string                `json:"title"`
	Description       string                `json:"description"`
	StartedAt         time.Time             `json:"started_at"`
	EndsAt            time.Time             `json:"ends_at"`
	MaxParticipants   int                   `json:"max_participants"`
	WinnersCount      int                   `json:"winners_count"`
	Status            models.GiveawayStatus `json:"status"`
	CreatedAt         time.Time             `json:"created_at"`
	UpdatedAt         time.Time             `json:"updated_at"`
	ParticipantsCount int64                 `json:"participants_count"`
	CanEdit           bool                  `json:"can_edit"`
	UserRole          string                `json:"user_role"`
	Prizes            []models.PrizePlace   `json:"prizes"`
	AutoDistribute    bool                  `json:"auto_distribute"`
	Winners           []models.Winner       `json:"winners"`
	AllowTickets      bool                  `json:"allow_tickets"`
}

// SwaggerGiveawayResponse represents the response body for giveaway operations with Swagger documentation
type SwaggerGiveawayResponse struct {
	ID                string                `json:"id" swaggertype:"string" description:"Unique identifier of the giveaway"`
	CreatorID         int64                 `json:"creator_id" description:"Telegram ID of the giveaway creator"`
	Title             string                `json:"title" swaggertype:"string" description:"Title of the giveaway"`
	Description       string                `json:"description" swaggertype:"string" description:"Description of the giveaway"`
	StartedAt         time.Time             `json:"started_at" description:"Start time of the giveaway"`
	EndsAt            time.Time             `json:"ends_at" description:"End time of the giveaway"`
	MaxParticipants   int                   `json:"max_participants" description:"Maximum number of participants (0 for unlimited)"`
	WinnersCount      int                   `json:"winners_count" description:"Number of winners"`
	Status            models.GiveawayStatus `json:"status" description:"Current status of the giveaway"`
	CreatedAt         time.Time             `json:"created_at" description:"Creation timestamp"`
	UpdatedAt         time.Time             `json:"updated_at" description:"Last update timestamp"`
	ParticipantsCount int64                 `json:"participants_count" description:"Current number of participants"`
	CanEdit           bool                  `json:"can_edit" description:"Whether the giveaway can be edited"`
	UserRole          string                `json:"user_role" description:"User's role in this giveaway (creator/participant)"`
	Prizes            []models.PrizePlace   `json:"prizes" description:"List of prizes"`
	AutoDistribute    bool                  `json:"auto_distribute" description:"Whether prizes are automatically distributed"`
	Winners           []models.Winner       `json:"winners" description:"List of winners (if giveaway is completed)"`
	AllowTickets      bool                  `json:"allow_tickets" description:"Whether tickets are allowed"`
}

// SwaggerGiveawayDetailedResponse represents a detailed giveaway response for Swagger documentation
type SwaggerGiveawayDetailedResponse struct {
	ID                string                `json:"id" swaggertype:"string" description:"Unique identifier of the giveaway"`
	CreatorID         int64                 `json:"creator_id" description:"Telegram ID of the giveaway creator"`
	CreatorUsername   string                `json:"creator_username" description:"Username of the giveaway creator"`
	Title             string                `json:"title" swaggertype:"string" description:"Title of the giveaway"`
	Description       string                `json:"description" swaggertype:"string" description:"Description of the giveaway"`
	StartedAt         time.Time             `json:"started_at" description:"Start time of the giveaway"`
	EndedAt           time.Time             `json:"ended_at" description:"End time of the giveaway"`
	Duration          int64                 `json:"duration" description:"Duration of the giveaway in seconds"`
	MaxParticipants   int                   `json:"max_participants" description:"Maximum number of participants"`
	ParticipantsCount int64                 `json:"participants_count" description:"Current number of participants"`
	WinnersCount      int                   `json:"winners_count" description:"Number of winners"`
	Status            models.GiveawayStatus `json:"status" description:"Current status of the giveaway"`
	CreatedAt         time.Time             `json:"created_at" description:"Creation timestamp"`
	UpdatedAt         time.Time             `json:"updated_at" description:"Last update timestamp"`
	Winners           []models.WinnerDetail `json:"winners,omitempty" description:"List of winners with details"`
	Prizes            []models.PrizeDetail  `json:"prizes" description:"List of prizes with details"`
	UserRole          string                `json:"user_role" description:"User role (creator, participant, winner)"`
	UserTickets       int                   `json:"user_tickets" description:"Number of user's tickets"`
	TotalTickets      int                   `json:"total_tickets" description:"Total number of tickets"`
}