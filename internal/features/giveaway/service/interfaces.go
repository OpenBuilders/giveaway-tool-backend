package service

import (
	"context"
	"giveaway-tool-backend/internal/features/giveaway/models"
)

// GiveawayService defines the interface for giveaway operations
type GiveawayService interface {
	Create(ctx context.Context, userID int64, input *models.GiveawayCreate) (*models.GiveawayResponse, error)
	Update(ctx context.Context, userID int64, giveawayID string, input *models.GiveawayUpdate) (*models.GiveawayResponse, error)
	Delete(ctx context.Context, userID int64, giveawayID string) error
	GetByID(ctx context.Context, giveawayID string) (*models.GiveawayResponse, error)
	GetByIDWithUser(ctx context.Context, giveawayID string, userID int64) (*models.GiveawayResponse, error)
	GetByCreator(ctx context.Context, userID int64) ([]*models.GiveawayResponse, error)
	Join(ctx context.Context, userID int64, giveawayID string) error
	GetParticipants(ctx context.Context, giveawayID string) ([]int64, error)
	GetPrizeTemplates(ctx context.Context) ([]*models.PrizeTemplate, error)
	CreateCustomPrize(ctx context.Context, input *models.CustomPrizeCreate) (*models.Prize, error)
	GetWinners(ctx context.Context, userID int64, giveawayID string) ([]models.Winner, error)
	AddTickets(ctx context.Context, userID int64, giveawayID string, count int64) error
	GetHistoricalGiveaways(ctx context.Context, userID int64) ([]*models.GiveawayResponse, error)
	MoveToHistory(ctx context.Context, userID int64, giveawayID string) error
	GetCreatedGiveaways(ctx context.Context, userID int64) ([]*models.GiveawayDetailedResponse, error)
	GetParticipatedGiveaways(ctx context.Context, userID int64) ([]*models.GiveawayDetailedResponse, error)
	GetCreatedGiveawaysHistory(ctx context.Context, userID int64) ([]*models.GiveawayDetailedResponse, error)
	GetParticipationHistory(ctx context.Context, userID int64) ([]*models.GiveawayDetailedResponse, error)
	GetTopGiveaways(ctx context.Context, limit int) ([]*models.GiveawayResponse, error)
	GetRequirementTemplates(ctx context.Context) ([]*models.RequirementTemplate, error)
	GetAllCreatedGiveaways(ctx context.Context, userID int64) ([]*models.GiveawayDetailedResponse, error)
	CancelGiveaway(ctx context.Context, userID int64, giveawayID string) error
	RecreateGiveaway(ctx context.Context, userID int64, giveawayID string) (*models.GiveawayResponse, error)
	CheckRequirements(ctx context.Context, userID int64, giveawayID string) (*models.RequirementsCheckResponse, error)
	ProcessPreWinnerList(ctx context.Context, userID int64, giveawayID string, userIDs []int64) (*models.PreWinnerListResponse, error)
	ValidatePreWinnerUsers(ctx context.Context, userID int64, giveawayID string, userIDs []int64) (*models.PreWinnerValidationResponse, error)
	HasCustomRequirements(ctx context.Context, giveawayID string) (bool, error)
	CompleteGiveawayWithCustomRequirements(ctx context.Context, userID int64, giveawayID string) (*models.CompleteWithCustomResponse, error)
	GetMyActiveGiveaways(ctx context.Context, userID int64) ([]*models.GiveawayResponse, error)
	GetMyGiveawaysHistory(ctx context.Context, userID int64) ([]*models.GiveawayResponse, error)
	GetMyAwaitingActionGiveaways(ctx context.Context, userID int64) ([]*models.GiveawayResponse, error)
}

// ExpirationServiceInterface defines the interface for handling expired giveaways
type ExpirationServiceInterface interface {
	Start()
	Stop()
	ProcessExpiredGiveaways() error
}

// CompletionServiceInterface defines the interface for handling completed giveaways
type CompletionServiceInterface interface {
	Start()
	Stop()
	processCompletedGiveaways() error
}

// QueueServiceInterface defines the interface for managing giveaway processing queue
type QueueServiceInterface interface {
	Start()
	Stop()
}
