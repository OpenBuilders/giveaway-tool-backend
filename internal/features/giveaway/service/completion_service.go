package service

import (
	"context"
	"errors"
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

func (s *CompletionService) Start() {
	s.logger.Printf("Starting completion service")
	s.wg.Add(2)

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

func (s *CompletionService) Stop() {
	s.logger.Printf("Stopping completion service")
	s.cancel()
	s.wg.Wait()
	s.logger.Printf("Completion service stopped")
}

func (s *CompletionService) processCompletedGiveaways() error {
	activeGiveaways, err := s.repo.GetActiveGiveaways(s.ctx)
	if err != nil {
		return fmt.Errorf("failed to get active giveaways: %w", err)
	}

	// Обрабатываем обычные активные гивы
	for _, giveawayID := range activeGiveaways {
		if _, exists := s.processing.LoadOrStore(giveawayID, true); exists {
			continue
		}

		go func(id string) {
			defer s.processing.Delete(id)

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

	// Обрабатываем гивы с Custom требованиями, у которых истекло 24 часа
	customGiveaways, err := s.repo.GetCustomGiveaways(s.ctx)
	if err != nil {
		return fmt.Errorf("failed to get custom giveaways: %w", err)
	}

	for _, giveawayID := range customGiveaways {
		if _, exists := s.processing.LoadOrStore(giveawayID, true); exists {
			continue
		}

		go func(id string) {
			defer s.processing.Delete(id)

			select {
			case s.semaphore <- struct{}{}:
				defer func() { <-s.semaphore }()
			case <-s.ctx.Done():
				return
			}

			if err := s.processCustomGiveawayWithRetry(id); err != nil {
				s.logger.Printf("Failed to process custom giveaway %s: %v", id, err)
			}
		}(giveawayID)
	}

	return nil
}

func (s *CompletionService) processGiveawayWithRetry(giveawayID string) error {
	var lastErr error
	for attempt := 1; attempt <= MaxRetries; attempt++ {
		lockKey := fmt.Sprintf("lock:giveaway:%s", giveawayID)
		if err := s.repo.AcquireLock(s.ctx, lockKey, LockTimeout); err != nil {
			if err == repository.ErrAlreadyLocked {
				return nil
			}
			lastErr = err
			continue
		}

		defer s.repo.ReleaseLock(s.ctx, lockKey)

		if err := s.processGiveaway(giveawayID); err != nil {
			if err == repository.ErrGiveawayNotFound {
				return err
			}
			lastErr = err
			time.Sleep(RetryDelay)
			continue
		}

		return nil
	}

	return fmt.Errorf("failed after %d attempts, last error: %w", MaxRetries, lastErr)
}

func (s *CompletionService) processGiveaway(giveawayID string) error {
	ctx, cancel := context.WithTimeout(s.ctx, ProcessingTimeout)
	defer cancel()

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	giveaway, err := s.repo.GetByIDWithLock(ctx, tx, giveawayID)
	if err != nil {
		return fmt.Errorf("failed to get giveaway: %w", err)
	}

	if !giveaway.HasEnded() {
		return nil
	}

	if giveaway.Status != models.GiveawayStatusActive {
		return nil
	}

	// Проверяем, есть ли кастомные требования
	hasCustomReqs := false
	if giveaway.Requirements != nil && len(giveaway.Requirements) > 0 {
		for _, req := range giveaway.Requirements {
			if req.IsCustom() {
				hasCustomReqs = true
				break
			}
		}
	}

	// Если есть кастомные требования, переводим в состояние ожидания ручной проверки
	if hasCustomReqs {
		s.logger.Printf("Giveaway %s has custom requirements, switching to custom status", giveawayID)

		// Обновляем статус на custom и устанавливаем время истечения (24 часа)
		giveaway.Status = models.GiveawayStatusCustom
		giveaway.UpdatedAt = time.Now()
		if err := s.repo.UpdateTx(ctx, tx, giveaway); err != nil {
			return fmt.Errorf("failed to update giveaway status to custom: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}

		// Отправляем уведомление создателю о необходимости загрузить pre-winner list
		go s.notifyCreatorAboutCustomRequirements(giveaway)

		return nil
	}

	participants, err := s.repo.GetParticipantsTx(ctx, tx, giveawayID)
	if err != nil {
		return fmt.Errorf("failed to get participants: %w", err)
	}

	if len(participants) == 0 {
		return s.handleNoParticipants(ctx, tx, giveaway)
	}

	// Adjust winners count if there are fewer participants
	if len(participants) < giveaway.WinnersCount {
		giveaway.WinnersCount = len(participants)
	}

	// Step 1: Select winners
	winners, err := s.selectWinners(ctx, tx, giveaway, participants)
	if err != nil {
		return fmt.Errorf("failed to select winners: %w", err)
	}

	// Step 2: Create win records and distribute prizes
	if err := s.createWinRecordsAndDistributePrizes(ctx, tx, giveaway, winners); err != nil {
		return fmt.Errorf("failed to create win records and distribute prizes: %w", err)
	}

	// Step 3: Update giveaway status
	giveaway.Status = models.GiveawayStatusCompleted
	giveaway.UpdatedAt = time.Now()
	if err := s.repo.UpdateTx(ctx, tx, giveaway); err != nil {
		return fmt.Errorf("failed to update giveaway status: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Step 4: Send notifications asynchronously
	go s.sendNotifications(giveaway, winners)

	s.logger.Printf("Successfully completed giveaway %s with %d winners", giveawayID, len(winners))
	return nil
}

func (s *CompletionService) handleNoParticipants(ctx context.Context, tx repository.Transaction, giveaway *models.Giveaway) error {
	giveaway.Status = models.GiveawayStatusCompleted
	giveaway.UpdatedAt = time.Now()

	if err := s.repo.UpdateTx(ctx, tx, giveaway); err != nil {
		return fmt.Errorf("failed to update giveaway status: %w", err)
	}

	if err := s.repo.AddToHistoryTx(ctx, tx, giveaway.ID); err != nil {
		return fmt.Errorf("failed to move giveaway to history: %w", err)
	}

	return tx.Commit()
}

func (s *CompletionService) selectWinners(ctx context.Context, tx repository.Transaction, giveaway *models.Giveaway, participants []int64) ([]models.Winner, error) {
	if !giveaway.AllowTickets {
		// Standard random selection
		return s.selectWinnersRandom(ctx, tx, giveaway, participants)
	}

	// Selection with tickets
	tickets, err := s.repo.GetAllTicketsTx(ctx, tx, giveaway.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tickets: %w", err)
	}

	return s.selectWinnersWithTickets(ctx, tx, giveaway, tickets)
}

func (s *CompletionService) selectWinnersRandom(ctx context.Context, tx repository.Transaction, giveaway *models.Giveaway, participants []int64) ([]models.Winner, error) {
	// Фильтруем участников по не-кастомным требованиям
	var validParticipants []int64

	for _, userID := range participants {
		// Проверяем требования (кроме кастомных)
		if len(giveaway.Requirements) > 0 {
			nonCustomReqs := make([]models.Requirement, 0)
			for _, req := range giveaway.Requirements {
				if !req.IsCustom() {
					nonCustomReqs = append(nonCustomReqs, req)
				}
			}

			if len(nonCustomReqs) > 0 {
				// Проверяем выполнение всех не-кастомных требований
				allMet := true
				for _, req := range nonCustomReqs {
					switch req.Type {
					case models.RequirementTypeSubscription:
						isMember, err := s.telegramClient.CheckMembership(ctx, userID, req.Username)
						if err != nil {
							// Если получили ошибку RPS, пропускаем проверку
							var rpsErr *telegram.RPSError
							if ok := errors.As(err, &rpsErr); ok {
								continue
							}
							allMet = false
							break
						}
						if !isMember {
							allMet = false
							break
						}

					case models.RequirementTypeBoost:
						hasBoost, err := s.telegramClient.CheckBoost(ctx, userID, req.Username)
						if err != nil {
							// Если получили ошибку RPS, пропускаем проверку
							var rpsErr *telegram.RPSError
							if ok := errors.As(err, &rpsErr); ok {
								continue
							}
							allMet = false
							break
						}
						if !hasBoost {
							allMet = false
							break
						}
					}
				}
				if allMet {
					validParticipants = append(validParticipants, userID)
				}
			} else {
				// Если нет не-кастомных требований, все участники валидны
				validParticipants = append(validParticipants, userID)
			}
		} else {
			// Если нет требований вообще, все участники валидны
			validParticipants = append(validParticipants, userID)
		}
	}

	// Если нет валидных участников, возвращаем пустой список
	if len(validParticipants) == 0 {
		return []models.Winner{}, nil
	}

	// Перемешиваем список валидных участников
	shuffled := make([]int64, len(validParticipants))
	copy(shuffled, validParticipants)

	// Fisher-Yates shuffle
	for i := len(shuffled) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	}

	// Выбираем первых winnersCount участников
	winnersCount := giveaway.WinnersCount
	if winnersCount > len(shuffled) {
		winnersCount = len(shuffled)
	}

	winners := make([]models.Winner, winnersCount)
	for i := 0; i < winnersCount; i++ {
		winners[i] = models.Winner{
			UserID:   shuffled[i],
			Username: fmt.Sprintf("user_%d", shuffled[i]), // Базовое имя, будет обновлено позже
			Place:    i + 1,
		}
	}

	return winners, nil
}

func (s *CompletionService) selectWinnersWithTickets(ctx context.Context, tx repository.Transaction, giveaway *models.Giveaway, tickets map[int64]int) ([]models.Winner, error) {
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

	winners := make([]models.Winner, 0, giveaway.WinnersCount)
	selectedUsers := make(map[int64]bool)

	for place := 1; place <= giveaway.WinnersCount; place++ {
		if len(ticketPool) == 0 {
			break
		}

		totalWeight := 0
		for _, ticket := range ticketPool {
			if !selectedUsers[ticket.UserID] {
				totalWeight += ticket.Weight
			}
		}

		if totalWeight == 0 {
			break
		}

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

		user, err := s.repo.GetUser(ctx, winnerUserID)
		if err != nil {
			s.logger.Printf("Failed to get user info for %d: %v", winnerUserID, err)
			// Remove this user from pool and continue
			for i, ticket := range ticketPool {
				if ticket.UserID == winnerUserID {
					ticketPool = append(ticketPool[:i], ticketPool[i+1:]...)
					break
				}
			}
			continue
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

func (s *CompletionService) createWinRecordsAndDistributePrizes(ctx context.Context, tx repository.Transaction, giveaway *models.Giveaway, winners []models.Winner) error {
	var customPrizes []models.Winner // Track custom prizes for creator notification

	for _, winner := range winners {
		// Get prize for this place
		prize, err := s.getPrizeForPlace(ctx, tx, giveaway, winner.Place)
		if err != nil {
			s.logger.Printf("Failed to get prize for place %d: %v", winner.Place, err)
			continue
		}

		// Create win record
		record := &models.WinRecord{
			ID:         uuid.New().String(),
			GiveawayID: giveaway.ID,
			UserID:     winner.UserID,
			PrizeID:    prize.ID,
			Place:      winner.Place,
			Status:     models.PrizeStatusPending,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err := s.repo.CreateWinRecordTx(ctx, tx, record); err != nil {
			return fmt.Errorf("failed to create win record: %w", err)
		}

		// Handle prize distribution based on type
		if prize.Type == models.PrizeTypeCustom {
			// Custom prizes are not distributed automatically
			customPrizes = append(customPrizes, winner)
		} else if giveaway.AutoDistribute && prize.IsInternal {
			// Auto-distribute internal prizes
			if err := s.repo.DistributePrizeTx(ctx, tx, giveaway.ID, winner.UserID, prize.ID); err != nil {
				s.logger.Printf("Failed to distribute prize %s to user %d: %v", prize.ID, winner.UserID, err)
			} else {
				now := time.Now()
				record.Status = models.PrizeStatusDistributed
				record.ReceivedAt = &now
				if err := s.repo.UpdateWinRecordTx(ctx, tx, record); err != nil {
					s.logger.Printf("Failed to update win record: %v", err)
				}
			}
		}
	}

	// Store custom prizes info for creator notification
	if len(customPrizes) > 0 {
		s.processing.Store(fmt.Sprintf("custom_prizes:%s", giveaway.ID), customPrizes)
	}

	return nil
}

func (s *CompletionService) getPrizeForPlace(ctx context.Context, tx repository.Transaction, giveaway *models.Giveaway, place int) (*models.Prize, error) {
	if place <= 0 || place > len(giveaway.Prizes) {
		return nil, fmt.Errorf("invalid place: %d", place)
	}

	prizePlace := giveaway.Prizes[place-1]
	prize, err := s.repo.GetPrizeTx(ctx, tx, prizePlace.PrizeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get prize %s: %w", prizePlace.PrizeID, err)
	}

	return prize, nil
}

func (s *CompletionService) sendNotifications(giveaway *models.Giveaway, winners []models.Winner) {
	// Send notifications to winners
	for _, winner := range winners {
		if err := s.notifyWinner(winner, giveaway); err != nil {
			s.logger.Printf("Failed to notify winner %d: %v", winner.UserID, err)
		}
	}

	// Check if there are custom prizes and notify creator
	if customPrizesData, exists := s.processing.Load(fmt.Sprintf("custom_prizes:%s", giveaway.ID)); exists {
		if customPrizes, ok := customPrizesData.([]models.Winner); ok {
			if err := s.notifyCreatorAboutCustomPrizes(giveaway, customPrizes); err != nil {
				s.logger.Printf("Failed to notify creator about custom prizes: %v", err)
			}
		}
		s.processing.Delete(fmt.Sprintf("custom_prizes:%s", giveaway.ID))
	}
}

func (s *CompletionService) notifyWinner(winner models.Winner, giveaway *models.Giveaway) error {
	prize, err := s.getPrizeForPlace(s.ctx, nil, giveaway, winner.Place)
	if err != nil {
		return fmt.Errorf("failed to get prize info: %w", err)
	}

	// Create PrizeDetail for the existing NotifyWinner method
	prizeDetail := models.PrizeDetail{
		Type:        prize.Type,
		Name:        prize.Name,
		Description: prize.Description,
		IsInternal:  prize.IsInternal,
		Status:      string(models.PrizeStatusPending),
	}

	// Use the existing NotifyWinner method
	return s.telegramClient.NotifyWinner(winner.UserID, giveaway, winner.Place, prizeDetail)
}

func (s *CompletionService) notifyCreatorAboutCustomPrizes(giveaway *models.Giveaway, customPrizes []models.Winner) error {
	if len(customPrizes) == 0 {
		return nil
	}

	// Use the new NotifyCreatorAboutCustomPrizes method
	return s.telegramClient.NotifyCreatorAboutCustomPrizes(giveaway.CreatorID, giveaway, customPrizes)
}

func (s *CompletionService) notifyCreatorAboutCustomRequirements(giveaway *models.Giveaway) {
	if err := s.telegramClient.NotifyCreatorAboutCustomRequirements(giveaway.CreatorID, giveaway); err != nil {
		s.logger.Printf("Failed to send custom requirements notification to creator %d: %v", giveaway.CreatorID, err)
	} else {
		s.logger.Printf("Successfully sent custom requirements notification to creator %d", giveaway.CreatorID)
	}
}

func getPlaceSuffix(place int) string {
	switch place {
	case 1:
		return "st"
	case 2:
		return "nd"
	case 3:
		return "rd"
	default:
		return "th"
	}
}

func (s *CompletionService) processCustomGiveawayWithRetry(giveawayID string) error {
	var lastErr error
	for attempt := 1; attempt <= MaxRetries; attempt++ {
		lockKey := fmt.Sprintf("lock:giveaway:%s", giveawayID)
		if err := s.repo.AcquireLock(s.ctx, lockKey, LockTimeout); err != nil {
			if err == repository.ErrAlreadyLocked {
				return nil
			}
			lastErr = err
			continue
		}

		defer s.repo.ReleaseLock(s.ctx, lockKey)

		if err := s.processCustomGiveaway(giveawayID); err != nil {
			if err == repository.ErrGiveawayNotFound {
				return err
			}
			lastErr = err
			time.Sleep(RetryDelay)
			continue
		}

		return nil
	}

	return fmt.Errorf("failed after %d attempts, last error: %w", MaxRetries, lastErr)
}

func (s *CompletionService) processCustomGiveaway(giveawayID string) error {
	ctx, cancel := context.WithTimeout(s.ctx, ProcessingTimeout)
	defer cancel()

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	giveaway, err := s.repo.GetByIDWithLock(ctx, tx, giveawayID)
	if err != nil {
		return fmt.Errorf("failed to get giveaway: %w", err)
	}

	// Проверяем, что гив в статусе custom
	if giveaway.Status != models.GiveawayStatusCustom {
		return nil
	}

	// Проверяем, прошло ли 24 часа с момента перехода в статус custom
	customDeadline := giveaway.UpdatedAt.Add(24 * time.Hour)
	if time.Now().Before(customDeadline) {
		return nil // Еще не истекло 24 часа
	}

	s.logger.Printf("Custom giveaway %s expired, selecting winners randomly", giveawayID)

	// Получаем всех участников
	participants, err := s.repo.GetParticipantsTx(ctx, tx, giveawayID)
	if err != nil {
		return fmt.Errorf("failed to get participants: %w", err)
	}

	if len(participants) == 0 {
		return s.handleNoParticipants(ctx, tx, giveaway)
	}

	// Adjust winners count if there are fewer participants
	if len(participants) < giveaway.WinnersCount {
		giveaway.WinnersCount = len(participants)
	}

	// Выбираем победителей случайным образом (только из тех, кто выполнил не-кастомные требования)
	winners, err := s.selectWinnersRandom(ctx, tx, giveaway, participants)
	if err != nil {
		return fmt.Errorf("failed to select winners: %w", err)
	}

	// Создаем записи о победах и распределяем призы
	if err := s.createWinRecordsAndDistributePrizes(ctx, tx, giveaway, winners); err != nil {
		return fmt.Errorf("failed to create win records and distribute prizes: %w", err)
	}

	// Обновляем статус гива
	giveaway.Status = models.GiveawayStatusCompleted
	giveaway.UpdatedAt = time.Now()
	if err := s.repo.UpdateTx(ctx, tx, giveaway); err != nil {
		return fmt.Errorf("failed to update giveaway status: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Отправляем уведомления
	go s.sendNotifications(giveaway, winners)

	s.logger.Printf("Successfully completed expired custom giveaway %s with %d winners", giveawayID, len(winners))
	return nil
}

// selectRandomWinners выбирает случайных победителей из списка участников
func (s *CompletionService) selectRandomWinners(participants []int64, winnersCount int) []int64 {
	if len(participants) <= winnersCount {
		return participants
	}

	// Перемешиваем участников
	shuffled := make([]int64, len(participants))
	copy(shuffled, participants)

	// Fisher-Yates shuffle
	for i := len(shuffled) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	}

	return shuffled[:winnersCount]
}

// handleCustomGiveawayExpiration обрабатывает истечение 24 часов для гива с кастомными требованиями
func (s *CompletionService) handleCustomGiveawayExpiration(giveaway *models.Giveaway) {
	ctx := context.Background()

	// Получаем всех участников гива
	participants, err := s.repo.GetParticipants(ctx, giveaway.ID)
	if err != nil {
		s.logger.Printf("Failed to get participants for custom giveaway expiration: %v", err)
		return
	}

	// Собираем участников, выполнивших не-кастомные требования
	var validUsers []int64
	for _, participantID := range participants {
		// Проверяем не-кастомные требования
		if len(giveaway.Requirements) > 0 {
			nonCustomReqs := make([]models.Requirement, 0)
			for _, req := range giveaway.Requirements {
				if !req.IsCustom() {
					nonCustomReqs = append(nonCustomReqs, req)
				}
			}

			if len(nonCustomReqs) > 0 {
				// Проверяем выполнение всех не-кастомных требований
				allMet := true
				for _, req := range nonCustomReqs {
					switch req.Type {
					case models.RequirementTypeSubscription:
						isMember, err := s.telegramClient.CheckMembership(ctx, participantID, req.Username)
						if err != nil {
							allMet = false
							break
						}
						if !isMember {
							allMet = false
							break
						}

					case models.RequirementTypeBoost:
						hasBoost, err := s.telegramClient.CheckBoost(ctx, participantID, req.Username)
						if err != nil {
							allMet = false
							break
						}
						if !hasBoost {
							allMet = false
							break
						}
					}
				}
				if !allMet {
					continue // Пропускаем участника, не выполнившего требования
				}
			}
		}

		validUsers = append(validUsers, participantID)
	}

	// Выбираем победителей случайным образом из всех валидных участников
	if len(validUsers) == 0 {
		s.logger.Printf("No valid participants found for custom giveaway expiration")

		// Обновляем статус гива на completed
		giveaway.Status = models.GiveawayStatusCompleted
		giveaway.UpdatedAt = time.Now()
		if err := s.repo.Update(ctx, giveaway); err != nil {
			s.logger.Printf("Failed to update giveaway status to completed: %v", err)
		}
		return
	}

	// Выбираем победителей
	winners := s.selectRandomWinners(validUsers, giveaway.WinnersCount)

	// Создаем записи о победах
	for i, winnerID := range winners {
		winRecord := &models.WinRecord{
			ID:         uuid.New().String(),
			GiveawayID: giveaway.ID,
			UserID:     winnerID,
			Place:      i + 1,
			Status:     models.PrizeStatusPending,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err := s.repo.CreateWinRecord(ctx, winRecord); err != nil {
			s.logger.Printf("Failed to create win record: %v", err)
			continue
		}

		// Отправляем уведомление победителю
		if i < len(giveaway.Prizes) {
			prizePlace := giveaway.Prizes[i]
			// Создаем PrizeDetail из PrizePlace
			prizeDetail := models.PrizeDetail{
				Type:        prizePlace.PrizeType,
				Name:        fmt.Sprintf("Prize for %d place", i+1),
				Description: fmt.Sprintf("Prize for %d place", i+1),
				IsInternal:  models.IsPrizeInternal(prizePlace.PrizeType),
				Status:      string(models.PrizeStatusPending),
			}
			if err := s.telegramClient.NotifyWinner(winnerID, giveaway, i+1, prizeDetail); err != nil {
				s.logger.Printf("Failed to notify winner: %v", err)
			}
		}
	}

	// Обновляем статус гива
	giveaway.Status = models.GiveawayStatusCompleted
	giveaway.UpdatedAt = time.Now()
	if err := s.repo.Update(ctx, giveaway); err != nil {
		s.logger.Printf("Failed to update giveaway status to completed: %v", err)
		return
	}

	s.logger.Printf("Custom giveaway automatically completed after 24 hours")
}
