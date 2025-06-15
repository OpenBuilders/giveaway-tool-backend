package repository

import (
	"context"
	"errors"
	"giveaway-tool-backend/internal/features/giveaway/models"
	usermodels "giveaway-tool-backend/internal/features/user/models"
	"time"
)

var (
	ErrGiveawayNotFound = errors.New("giveaway not found")
	ErrLockTimeout      = errors.New("failed to acquire lock: timeout")
	ErrAlreadyLocked    = errors.New("resource is already locked")
)

type Transaction interface {
	Commit() error
	Rollback() error
}

type GiveawayRepository interface {
	BeginTx(ctx context.Context) (Transaction, error)

	Create(ctx context.Context, giveaway *models.Giveaway) error
	GetByID(ctx context.Context, id string) (*models.Giveaway, error)
	GetByIDWithLock(ctx context.Context, tx Transaction, id string) (*models.Giveaway, error)
	Update(ctx context.Context, giveaway *models.Giveaway) error
	UpdateTx(ctx context.Context, tx Transaction, giveaway *models.Giveaway) error
	Delete(ctx context.Context, id string) error

	GetActiveGiveaways(ctx context.Context) ([]string, error)
	GetPendingGiveaways(ctx context.Context) ([]*models.Giveaway, error)
	AddToPending(ctx context.Context, id string) error
	AddToPendingTx(ctx context.Context, tx Transaction, id string) error
	AddToHistory(ctx context.Context, id string) error
	AddToHistoryTx(ctx context.Context, tx Transaction, id string) error
	MoveToHistory(ctx context.Context, id string) error
	UpdateStatusAtomic(ctx context.Context, tx Transaction, id string, update models.GiveawayStatusUpdate) error
	UpdateStatus(ctx context.Context, id string, status models.GiveawayStatus) error
	UpdateStatusIfPending(ctx context.Context, id string, status models.GiveawayStatus) (bool, error)

	AddParticipant(ctx context.Context, giveawayID string, userID int64) error
	GetParticipants(ctx context.Context, giveawayID string) ([]int64, error)
	GetParticipantsTx(ctx context.Context, tx Transaction, giveawayID string) ([]int64, error)
	GetParticipantsCount(ctx context.Context, giveawayID string) (int64, error)
	IsParticipant(ctx context.Context, giveawayID string, userID int64) (bool, error)

	SelectWinners(ctx context.Context, giveawayID string, count int) ([]models.Winner, error)
	SelectWinnersTx(ctx context.Context, tx Transaction, giveawayID string, count int) ([]models.Winner, error)
	GetWinners(ctx context.Context, giveawayID string) ([]models.Winner, error)

	CreatePrize(ctx context.Context, prize *models.Prize) error
	GetPrize(ctx context.Context, id string) (*models.Prize, error)
	GetPrizeTx(ctx context.Context, tx Transaction, id string) (*models.Prize, error)
	GetPrizes(ctx context.Context, giveawayID string) ([]models.PrizePlace, error)
	GetPrizesTx(ctx context.Context, tx Transaction, giveawayID string) ([]models.PrizePlace, error)
	AssignPrizeTx(ctx context.Context, tx Transaction, userID int64, prizeID string, place int) error
	GetPrizeTemplates(ctx context.Context) ([]*models.PrizeTemplate, error)

	GetRequirementTemplates(ctx context.Context) ([]*models.RequirementTemplate, error)

	AddTickets(ctx context.Context, giveawayID string, userID int64, count int64) error
	GetUserTickets(ctx context.Context, giveawayID string, userID int64) (int, error)
	GetTotalTickets(ctx context.Context, giveawayID string) (int, error)
	GetAllTicketsTx(ctx context.Context, tx Transaction, giveawayID string) (map[int64]int, error)

	CreateWinRecord(ctx context.Context, record *models.WinRecord) error
	CreateWinRecordTx(ctx context.Context, tx Transaction, record *models.WinRecord) error
	GetWinRecord(ctx context.Context, id string) (*models.WinRecord, error)
	GetWinRecordsByGiveaway(ctx context.Context, giveawayID string) ([]*models.WinRecord, error)
	UpdateWinRecord(ctx context.Context, record *models.WinRecord) error
	UpdateWinRecordTx(ctx context.Context, tx Transaction, record *models.WinRecord) error
	DistributePrizeTx(ctx context.Context, tx Transaction, giveawayID string, userID int64, prizeID string) error

	GetCreator(ctx context.Context, userID int64) (*usermodels.User, error)
	GetUser(ctx context.Context, userID int64) (*usermodels.User, error)

	RemoveFromActive(ctx context.Context, id string) error
	DeleteParticipantsCount(ctx context.Context, id string) error
	DeletePrizes(ctx context.Context, id string) error

	GetByCreatorAndStatus(ctx context.Context, userID int64, statuses []models.GiveawayStatus) ([]*models.Giveaway, error)
	GetByParticipantAndStatus(ctx context.Context, userID int64, statuses []models.GiveawayStatus) ([]*models.Giveaway, error)

	AcquireLock(ctx context.Context, key string, timeout time.Duration) error
	ReleaseLock(ctx context.Context, key string) error

	CleanupInconsistentData(ctx context.Context) error

	AddToProcessingSet(ctx context.Context, id string) bool
	RemoveFromProcessingSet(ctx context.Context, id string) error
	GetExpiredGiveaways(ctx context.Context, now int64) ([]string, error)

	GetTopGiveaways(ctx context.Context, limit int) ([]*models.Giveaway, error)

	CancelGiveaway(ctx context.Context, giveawayID string) error

	SetChannelAvatar(ctx context.Context, channelID string, avatarURL string) error
	GetChannelAvatar(ctx context.Context, channelID string) (string, error)
}
