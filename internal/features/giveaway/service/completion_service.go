package service

import (
	"context"
	"fmt"
	"giveaway-tool-backend/internal/features/giveaway/models"
	"giveaway-tool-backend/internal/features/giveaway/repository"
	"giveaway-tool-backend/internal/platform/telegram"
	"log"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
)

// CompletionService отвечает за обработку завершения розыгрышей
type CompletionService struct {
	ctx            context.Context
	cancel         context.CancelFunc
	repo           repository.GiveawayRepository
	logger         *log.Logger
	processing     sync.Map
	semaphore      chan struct{}
	wg             sync.WaitGroup
	telegramClient *telegram.Client
}

// NewCompletionService создает новый сервис обработки завершения розыгрышей
func NewCompletionService(repo repository.GiveawayRepository, telegramClient *telegram.Client) *CompletionService {
	ctx, cancel := context.WithCancel(context.Background())
	return &CompletionService{
		ctx:            ctx,
		cancel:         cancel,
		repo:           repo,
		logger:         log.New(os.Stdout, "[CompletionService] ", log.LstdFlags),
		semaphore:      make(chan struct{}, MaxConcurrentProcessing),
		telegramClient: telegramClient,
	}
}

// Start запускает сервис
func (s *CompletionService) Start() {
	s.logger.Printf("Starting completion service")
	s.wg.Add(2)

	// Запускаем обработку завершенных розыгрышей
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(CheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := s.processCompletedGiveaways(); err != nil {
					s.logger.Printf("Error processing completed giveaways: %v", err)
				}
			case <-s.ctx.Done():
				return
			}
		}
	}()

	// Запускаем периодическую очистку
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(CleanupInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := s.repo.CleanupInconsistentData(s.ctx); err != nil {
					s.logger.Printf("Error cleaning up inconsistent data: %v", err)
				}
			case <-s.ctx.Done():
				return
			}
		}
	}()
}

// Stop останавливает сервис
func (s *CompletionService) Stop() {
	s.logger.Printf("Stopping completion service")
	s.cancel()
	s.wg.Wait()
	s.logger.Printf("Completion service stopped")
}

// processCompletedGiveaways обрабатывает завершенные розыгрыши
func (s *CompletionService) processCompletedGiveaways() error {
	// Получаем список активных розыгрышей
	activeGiveaways, err := s.repo.GetActiveGiveaways(s.ctx)
	if err != nil {
		return fmt.Errorf("failed to get active giveaways: %w", err)
	}

	for _, giveawayID := range activeGiveaways {
		// Проверяем, не обрабатывается ли уже этот розыгрыш
		if _, exists := s.processing.LoadOrStore(giveawayID, true); exists {
			continue
		}

		// Запускаем обработку в отдельной горутине
		go func(id string) {
			defer s.processing.Delete(id)

			// Получаем слот в семафоре
			select {
			case s.semaphore <- struct{}{}:
				defer func() { <-s.semaphore }()
			case <-s.ctx.Done():
				return
			}

			if err := s.processGiveawayWithRetry(id); err != nil {
				s.logger.Printf("Failed to process giveaway %s: %v", id, err)
			}
		}(giveawayID)
	}

	return nil
}

// processGiveawayWithRetry обрабатывает розыгрыш с повторными попытками
func (s *CompletionService) processGiveawayWithRetry(giveawayID string) error {
	var lastErr error
	for attempt := 1; attempt <= MaxRetries; attempt++ {
		// Пытаемся получить блокировку
		lockKey := fmt.Sprintf("lock:giveaway:%s", giveawayID)
		if err := s.repo.AcquireLock(s.ctx, lockKey, LockTimeout); err != nil {
			if err == repository.ErrAlreadyLocked {
				return nil // Розыгрыш обрабатывается другим процессом
			}
			lastErr = err
			continue
		}

		// Гарантируем освобождение блокировки
		defer s.repo.ReleaseLock(s.ctx, lockKey)

		// Обрабатываем розыгрыш
		if err := s.processGiveaway(giveawayID); err != nil {
			if err == repository.ErrGiveawayNotFound {
				return err // Не пытаемся повторить, если розыгрыш не найден
			}
			lastErr = err
			time.Sleep(RetryDelay)
			continue
		}

		return nil
	}

	return fmt.Errorf("failed after %d attempts, last error: %w", MaxRetries, lastErr)
}

// processGiveaway обрабатывает один розыгрыш
func (s *CompletionService) processGiveaway(giveawayID string) error {
	// Создаем контекст с таймаутом
	ctx, cancel := context.WithTimeout(s.ctx, ProcessingTimeout)
	defer cancel()

	// Начинаем транзакцию
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Получаем информацию о розыгрыше с блокировкой
	giveaway, err := s.repo.GetByIDWithLock(ctx, tx, giveawayID)
	if err != nil {
		return fmt.Errorf("failed to get giveaway: %w", err)
	}

	// Проверяем, завершился ли розыгрыш
	if !giveaway.HasEnded() {
		return nil
	}

	// Проверяем статус
	if giveaway.Status != models.GiveawayStatusActive {
		return nil
	}

	// Получаем участников
	participants, err := s.repo.GetParticipantsTx(ctx, tx, giveawayID)
	if err != nil {
		return fmt.Errorf("failed to get participants: %w", err)
	}

	// Обрабатываем случай отсутствия участников
	if len(participants) == 0 {
		return s.handleNoParticipants(ctx, tx, giveaway)
	}

	// Корректируем количество победителей при необходимости
	if len(participants) < giveaway.WinnersCount {
		giveaway.WinnersCount = len(participants)
	}

	// Выбираем победителей
	winners, err := s.selectWinners(ctx, tx, giveaway)
	if err != nil {
		return fmt.Errorf("failed to select winners: %w", err)
	}

	// Создаем записи о выигрышах и распределяем призы
	if err := s.createWinRecords(ctx, tx, giveaway, winners); err != nil {
		return fmt.Errorf("failed to create win records: %w", err)
	}

	// Обновляем статус розыгрыша
	giveaway.Status = models.GiveawayStatusCompleted
	giveaway.UpdatedAt = time.Now()
	if err := s.repo.UpdateTx(ctx, tx, giveaway); err != nil {
		return fmt.Errorf("failed to update giveaway status: %w", err)
	}

	// Перемещаем розыгрыш в историю
	if err := s.repo.AddToHistoryTx(ctx, tx, giveaway.ID); err != nil {
		return fmt.Errorf("failed to move giveaway to history: %w", err)
	}

	// Фиксируем транзакцию
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Асинхронно отправляем уведомления
	go s.notifyWinners(winners, giveaway)

	return nil
}

// handleNoParticipants обрабатывает случай отсутствия участников
func (s *CompletionService) handleNoParticipants(ctx context.Context, tx repository.Transaction, giveaway *models.Giveaway) error {
	giveaway.Status = models.GiveawayStatusCancelled
	giveaway.UpdatedAt = time.Now()

	if err := s.repo.UpdateTx(ctx, tx, giveaway); err != nil {
		return fmt.Errorf("failed to update giveaway status: %w", err)
	}

	// Перемещаем розыгрыш в историю
	if err := s.repo.AddToHistoryTx(ctx, tx, giveaway.ID); err != nil {
		return fmt.Errorf("failed to move giveaway to history: %w", err)
	}

	return tx.Commit()
}

// selectWinners выбирает победителей с учетом билетов
func (s *CompletionService) selectWinners(ctx context.Context, tx repository.Transaction, giveaway *models.Giveaway) ([]models.Winner, error) {
	if !giveaway.AllowTickets {
		return s.repo.SelectWinnersTx(ctx, tx, giveaway.ID, giveaway.WinnersCount)
	}

	tickets, err := s.repo.GetAllTicketsTx(ctx, tx, giveaway.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tickets: %w", err)
	}

	return s.selectWinnersWithTickets(ctx, tx, giveaway, tickets)
}

// createWinRecords создает записи о выигрышах и распределяет призы
func (s *CompletionService) createWinRecords(ctx context.Context, tx repository.Transaction, giveaway *models.Giveaway, winners []models.Winner) error {
	for _, winner := range winners {
		prizePlace := giveaway.Prizes[winner.Place-1]

		record := &models.WinRecord{
			ID:         uuid.New().String(),
			GiveawayID: giveaway.ID,
			UserID:     winner.UserID,
			PrizeID:    prizePlace.PrizeID,
			Place:      winner.Place,
			Status:     models.PrizeStatusPending,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err := s.repo.CreateWinRecordTx(ctx, tx, record); err != nil {
			return fmt.Errorf("failed to create win record: %w", err)
		}

		if giveaway.AutoDistribute {
			prize, err := s.repo.GetPrizeTx(ctx, tx, record.PrizeID)
			if err != nil {
				return fmt.Errorf("failed to get prize: %w", err)
			}

			if prize.IsInternal {
				if err := s.repo.DistributePrizeTx(ctx, tx, giveaway.ID, winner.UserID, prize.ID); err != nil {
					return fmt.Errorf("failed to distribute prize: %w", err)
				}

				now := time.Now()
				record.Status = models.PrizeStatusDistributed
				record.ReceivedAt = &now
				if err := s.repo.UpdateWinRecordTx(ctx, tx, record); err != nil {
					return fmt.Errorf("failed to update win record: %w", err)
				}
			}
		}
	}

	return nil
}

// selectWinnersWithTickets выбирает победителей с учетом билетов
func (s *CompletionService) selectWinnersWithTickets(ctx context.Context, tx repository.Transaction, giveaway *models.Giveaway, tickets map[int64]int) ([]models.Winner, error) {
	// Создаем пул билетов
	var ticketPool []struct {
		UserID int64
		Weight int
	}

	for userID, count := range tickets {
		ticketPool = append(ticketPool, struct {
			UserID int64
			Weight int
		}{
			UserID: userID,
			Weight: count,
		})
	}

	// Выбираем победителей
	winners := make([]models.Winner, 0, giveaway.WinnersCount)
	selectedUsers := make(map[int64]bool)

	for place := 1; place <= giveaway.WinnersCount; place++ {
		if len(ticketPool) == 0 {
			break
		}

		// Вычисляем общий вес
		totalWeight := 0
		for _, ticket := range ticketPool {
			if !selectedUsers[ticket.UserID] {
				totalWeight += ticket.Weight
			}
		}

		if totalWeight == 0 {
			break
		}

		// Выбираем победителя
		winningTicket := rand.Intn(totalWeight) + 1
		currentWeight := 0
		var winnerUserID int64

		for _, ticket := range ticketPool {
			if selectedUsers[ticket.UserID] {
				continue
			}
			currentWeight += ticket.Weight
			if currentWeight >= winningTicket {
				winnerUserID = ticket.UserID
				break
			}
		}

		// Получаем информацию о пользователе
		user, err := s.repo.GetUser(ctx, winnerUserID)
		if err != nil {
			return nil, fmt.Errorf("failed to get user info: %w", err)
		}

		winners = append(winners, models.Winner{
			UserID:   winnerUserID,
			Username: user.Username,
			Place:    place,
		})
		selectedUsers[winnerUserID] = true
	}

	return winners, nil
}

// notifyWinners отправляет уведомления победителям
func (s *CompletionService) notifyWinners(winners []models.Winner, giveaway *models.Giveaway) {
	for _, winner := range winners {
		if err := s.notifyWinner(winner, giveaway); err != nil {
			s.logger.Printf("Failed to notify winner %d: %v", winner.UserID, err)
		}
	}
}

// notifyWinner отправляет уведомление одному победителю
func (s *CompletionService) notifyWinner(winner models.Winner, giveaway *models.Giveaway) error {
	// Проверяем валидность места
	if winner.Place <= 0 || winner.Place > len(giveaway.Prizes) {
		return fmt.Errorf("invalid place: %d", winner.Place)
	}

	// Получаем информацию о призе из базы данных
	prizePlace := giveaway.Prizes[winner.Place-1]
	prize, err := s.repo.GetPrize(s.ctx, prizePlace.PrizeID)
	if err != nil {
		return fmt.Errorf("failed to get prize info: %w", err)
	}

	prizeDetail := models.PrizeDetail{
		Type:        prize.Type,
		Name:        prize.Name,
		Description: prize.Description,
		IsInternal:  prize.IsInternal,
		Status:      string(models.PrizeStatusPending),
	}

	return s.telegramClient.NotifyWinner(winner.UserID, giveaway, winner.Place, prizeDetail)
}
