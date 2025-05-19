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

// Transaction представляет интерфейс транзакции
type Transaction interface {
	Commit() error
	Rollback() error
}

// GiveawayRepository определяет интерфейс для работы с розыгрышами
type GiveawayRepository interface {
	// Транзакции
	BeginTx(ctx context.Context) (Transaction, error)

	// Основные операции с розыгрышами
	Create(ctx context.Context, giveaway *models.Giveaway) error
	GetByID(ctx context.Context, id string) (*models.Giveaway, error)
	GetByIDWithLock(ctx context.Context, tx Transaction, id string) (*models.Giveaway, error)
	Update(ctx context.Context, giveaway *models.Giveaway) error
	UpdateTx(ctx context.Context, tx Transaction, giveaway *models.Giveaway) error
	Delete(ctx context.Context, id string) error

	// Операции со статусами
	GetActiveGiveaways(ctx context.Context) ([]string, error)
	GetPendingGiveaways(ctx context.Context) ([]*models.Giveaway, error)
	AddToPending(ctx context.Context, id string) error
	AddToPendingTx(ctx context.Context, tx Transaction, id string) error
	AddToHistory(ctx context.Context, id string) error
	AddToHistoryTx(ctx context.Context, tx Transaction, id string) error
	MoveToHistory(ctx context.Context, id string) error
	UpdateStatusAtomic(ctx context.Context, tx Transaction, id string, update models.GiveawayStatusUpdate) error

	// Операции с участниками
	AddParticipant(ctx context.Context, giveawayID string, userID int64) error
	GetParticipants(ctx context.Context, giveawayID string) ([]int64, error)
	GetParticipantsTx(ctx context.Context, tx Transaction, giveawayID string) ([]int64, error)
	GetParticipantsCount(ctx context.Context, giveawayID string) (int64, error)
	IsParticipant(ctx context.Context, giveawayID string, userID int64) (bool, error)

	// Операции с победителями
	SelectWinners(ctx context.Context, giveawayID string, count int) ([]models.Winner, error)
	SelectWinnersTx(ctx context.Context, tx Transaction, giveawayID string, count int) ([]models.Winner, error)
	GetWinners(ctx context.Context, giveawayID string) ([]models.Winner, error)

	// Операции с призами
	CreatePrize(ctx context.Context, prize *models.Prize) error
	GetPrize(ctx context.Context, id string) (*models.Prize, error)
	GetPrizeTx(ctx context.Context, tx Transaction, id string) (*models.Prize, error)
	GetPrizes(ctx context.Context, giveawayID string) ([]models.PrizePlace, error)
	GetPrizesTx(ctx context.Context, tx Transaction, giveawayID string) ([]models.PrizePlace, error)
	AssignPrizeTx(ctx context.Context, tx Transaction, userID int64, prizeID string, place int) error
	GetPrizeTemplates(ctx context.Context) ([]*models.PrizeTemplate, error)

	// Операции с билетами
	AddTickets(ctx context.Context, giveawayID string, userID int64, count int64) error
	GetUserTickets(ctx context.Context, giveawayID string, userID int64) (int, error)
	GetTotalTickets(ctx context.Context, giveawayID string) (int, error)
	GetAllTicketsTx(ctx context.Context, tx Transaction, giveawayID string) (map[int64]int, error)

	// Операции с записями о выигрышах
	CreateWinRecord(ctx context.Context, record *models.WinRecord) error
	CreateWinRecordTx(ctx context.Context, tx Transaction, record *models.WinRecord) error
	GetWinRecord(ctx context.Context, id string) (*models.WinRecord, error)
	GetWinRecordsByGiveaway(ctx context.Context, giveawayID string) ([]*models.WinRecord, error)
	UpdateWinRecord(ctx context.Context, record *models.WinRecord) error
	UpdateWinRecordTx(ctx context.Context, tx Transaction, record *models.WinRecord) error
	DistributePrizeTx(ctx context.Context, tx Transaction, giveawayID string, userID int64, prizeID string) error

	// Операции с пользователями
	GetCreator(ctx context.Context, userID int64) (*usermodels.User, error)
	GetUser(ctx context.Context, userID int64) (*usermodels.User, error)

	// Очистка данных
	RemoveFromActive(ctx context.Context, id string) error
	DeleteParticipantsCount(ctx context.Context, id string) error
	DeletePrizes(ctx context.Context, id string) error

	// Получение розыгрышей по статусам
	GetByCreatorAndStatus(ctx context.Context, userID int64, statuses []models.GiveawayStatus) ([]*models.Giveaway, error)
	GetByParticipantAndStatus(ctx context.Context, userID int64, statuses []models.GiveawayStatus) ([]*models.Giveaway, error)

	// Блокировки
	AcquireLock(ctx context.Context, key string, timeout time.Duration) error
	ReleaseLock(ctx context.Context, key string) error

	// Очистка несогласованных данных
	CleanupInconsistentData(ctx context.Context) error

	// Операции с множеством обрабатываемых розыгрышей
	AddToProcessingSet(ctx context.Context, id string) bool
	RemoveFromProcessingSet(ctx context.Context, id string) error
	GetExpiredGiveaways(ctx context.Context, now int64) ([]string, error)

	// Новый метод для получения топ-розыгрышей
	GetTopGiveaways(ctx context.Context, limit int) ([]*models.Giveaway, error)
}
