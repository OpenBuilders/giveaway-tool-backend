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

	if err := s.repo.AddToHistoryTx(ctx, tx, giveaway.ID); err != nil {
		return fmt.Errorf("failed to move giveaway to history: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Step 4: Send notifications (asynchronously)
	go s.sendNotifications(giveaway, winners)

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
	winners := make([]models.Winner, 0, giveaway.WinnersCount)
	selectedUsers := make(map[int64]bool)

	for place := 1; place <= giveaway.WinnersCount; place++ {
		if len(participants) == 0 {
			break
		}

		// Select random participant
		idx := rand.Intn(len(participants))
		userID := participants[idx]

		// Get user info
		user, err := s.repo.GetUser(ctx, userID)
		if err != nil {
			s.logger.Printf("Failed to get user info for %d: %v", userID, err)
			// Remove this participant and continue
			participants = append(participants[:idx], participants[idx+1:]...)
			continue
		}

		winners = append(winners, models.Winner{
			UserID:   userID,
			Username: user.Username,
			Place:    place,
		})
		selectedUsers[userID] = true

		// Remove selected participant
		participants = append(participants[:idx], participants[idx+1:]...)
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
