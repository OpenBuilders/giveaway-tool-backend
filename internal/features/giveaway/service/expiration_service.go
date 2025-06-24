package service

import (
	"context"
	"fmt"
	"giveaway-tool-backend/internal/common/config"
	"giveaway-tool-backend/internal/features/giveaway/models"
	"giveaway-tool-backend/internal/features/giveaway/repository"
	"giveaway-tool-backend/internal/platform/telegram"
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

const (
	maxRetries     = 3
	retryInterval  = time.Second
	cleanupTimeout = 30 * time.Minute // Cleanup interval for inconsistent data
	lockTimeout    = 30 * time.Second
	// Maximum number of concurrent giveaways being processed
	maxConcurrentProcessing = 10
	// Timeout for processing a single giveaway
	giveawayProcessingTimeout = 2 * time.Minute
)

type ExpirationService struct {
	ctx        context.Context
	cancel     context.CancelFunc
	repo       repository.GiveawayRepository
	config     *config.Config
	logger     *log.Logger
	processing sync.Map
	wg         sync.WaitGroup
	// Semaphore to limit concurrent processing
	processSemaphore chan struct{}
	metrics          *metrics
	telegramClient   *telegram.Client
}

type metrics struct {
	ProcessingErrors   *counter
	ProcessingDuration *histogram
}

type counter struct {
	value int64
}

func (c *counter) Inc() {
	atomic.AddInt64(&c.value, 1)
}

type histogram struct {
	values []float64
	mu     sync.Mutex
}

func (h *histogram) Observe(value float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.values = append(h.values, value)
}

func NewExpirationService(repo repository.GiveawayRepository, config *config.Config, logger *log.Logger) *ExpirationService {
	ctx, cancel := context.WithCancel(context.Background())
	return &ExpirationService{
		ctx:              ctx,
		cancel:           cancel,
		repo:             repo,
		config:           config,
		logger:           logger,
		processSemaphore: make(chan struct{}, 10), // MaxConcurrentProcessing
		metrics: &metrics{
			ProcessingErrors:   &counter{},
			ProcessingDuration: &histogram{values: make([]float64, 0)},
		},
		telegramClient: telegram.NewClient(),
	}
}

func (s *ExpirationService) Start() {
	s.logger.Printf("Starting expiration service")
	s.wg.Add(2) // Add counter for two goroutines

	// Start processing expired giveaways
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := s.ProcessExpiredGiveaways(); err != nil {
					s.logger.Printf("Error processing expired giveaways: %v", err)
				}
			case <-s.ctx.Done():
				return
			}
		}
	}()

	// Start periodic cleanup of inconsistent data
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(30 * time.Minute) // cleanupTimeout
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

func (s *ExpirationService) Stop() {
	s.logger.Printf("Stopping expiration service")
	s.cancel()
	s.wg.Wait()
	s.logger.Printf("Expiration service stopped")
}

func (s *ExpirationService) ProcessExpiredGiveaways() error {
	ctx := context.Background()
	now := time.Now().Unix()

	// Get only expired giveaways
	expiredGiveaways, err := s.repo.GetExpiredGiveaways(ctx, now)
	if err != nil {
		s.logger.Printf("Failed to get expired giveaways: %v", err)
		return err
	}

	for _, giveawayID := range expiredGiveaways {
		// Try to add to processing
		if !s.repo.AddToProcessingSet(ctx, giveawayID) {
			s.logger.Printf("Giveaway %s is already being processed", giveawayID)
			continue
		}

		// Process in goroutine
		s.wg.Add(1)
		go func(id string) {
			defer s.wg.Done()
			defer s.repo.RemoveFromProcessingSet(ctx, id)

			if err := s.processGiveawayWithRetry(ctx, id); err != nil {
				s.logger.Printf("Failed to process giveaway %s: %v", id, err)
				s.metrics.ProcessingErrors.Inc()
			}
		}(giveawayID)
	}

	return nil
}

func (s *ExpirationService) processGiveawayWithRetry(ctx context.Context, giveawayID string) error {
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ { // maxRetries
		if err := s.processGiveaway(ctx, giveawayID); err != nil {
			lastErr = err
			s.logger.Printf("Attempt %d failed: %v", attempt, err)
			time.Sleep(time.Second * time.Duration(attempt)) // retryInterval
			continue
		}
		return nil
	}
	return fmt.Errorf("failed after %d attempts: %w", 3, lastErr)
}

func (s *ExpirationService) processGiveaway(ctx context.Context, giveawayID string) error {
	start := time.Now()
	defer func() {
		s.metrics.ProcessingDuration.Observe(time.Since(start).Seconds())
	}()

	// Begin transaction
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Update status to processing
	if err := s.repo.UpdateStatusAtomic(ctx, tx, giveawayID, models.GiveawayStatusUpdate{
		GiveawayID: giveawayID,
		OldStatus:  models.GiveawayStatusActive,
		NewStatus:  models.GiveawayStatusPending,
		UpdatedAt:  time.Now(),
		Reason:     "Processing expired giveaway",
	}); err != nil {
		return fmt.Errorf("failed to update status to pending: %w", err)
	}

	// Select winners with tickets
	winners, err := s.selectWinnersWithTickets(ctx, tx, giveawayID)
	if err != nil {
		return fmt.Errorf("failed to select winners: %w", err)
	}

	// Distribute prizes
	if err := s.distributePrizes(ctx, tx, giveawayID, winners); err != nil {
		return fmt.Errorf("failed to distribute prizes: %w", err)
	}

	// Update status to completed
	if err := s.repo.UpdateStatusAtomic(ctx, tx, giveawayID, models.GiveawayStatusUpdate{
		GiveawayID: giveawayID,
		OldStatus:  models.GiveawayStatusPending,
		NewStatus:  models.GiveawayStatusCompleted,
		UpdatedAt:  time.Now(),
		Reason:     "Giveaway completed successfully",
	}); err != nil {
		return fmt.Errorf("failed to update status to completed: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Asynchronously notify winners
	go s.notifyWinners(winners, giveawayID)

	return nil
}

func (s *ExpirationService) selectWinnersWithTickets(ctx context.Context, tx repository.Transaction, giveawayID string) ([]models.Winner, error) {
	s.logger.Printf("Selecting winners for giveaway %s", giveawayID)

	// Get giveaway information
	giveaway, err := s.repo.GetByIDWithLock(ctx, tx, giveawayID)
	if err != nil {
		return nil, fmt.Errorf("failed to get giveaway: %w", err)
	}

	// Get participants
	participants, err := s.repo.GetParticipantsTx(ctx, tx, giveawayID)
	if err != nil {
		return nil, fmt.Errorf("failed to get participants: %w", err)
	}

	s.logger.Printf("Found %d participants for giveaway %s", len(participants), giveawayID)

	if !giveaway.AllowTickets {
		winners, err := s.repo.SelectWinnersTx(ctx, tx, giveawayID, giveaway.WinnersCount)
		if err != nil {
			return nil, fmt.Errorf("failed to select winners: %w", err)
		}
		s.logger.Printf("Selected %d winners for giveaway %s (no tickets)", len(winners), giveawayID)
		return winners, nil
	}

	// Get tickets
	tickets, err := s.repo.GetAllTicketsTx(ctx, tx, giveawayID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tickets: %w", err)
	}

	s.logger.Printf("Found tickets for %d users in giveaway %s", len(tickets), giveawayID)

	// Create ticket pool
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

	// Select winners
	winners := make([]models.Winner, 0, giveaway.WinnersCount)
	selectedUsers := make(map[int64]bool)

	for place := 1; place <= giveaway.WinnersCount; place++ {
		if len(ticketPool) == 0 {
			break
		}

		// Calculate total weight
		totalWeight := 0
		for _, ticket := range ticketPool {
			if !selectedUsers[ticket.UserID] {
				totalWeight += ticket.Weight
			}
		}

		if totalWeight == 0 {
			break
		}

		// Select winner
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

		// Get user information
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

	s.logger.Printf("Selected %d winners for giveaway %s (with tickets)", len(winners), giveawayID)
	return winners, nil
}

func (s *ExpirationService) distributePrizes(ctx context.Context, tx repository.Transaction, giveawayID string, winners []models.Winner) error {
	// Get giveaway information
	giveaway, err := s.repo.GetByID(ctx, giveawayID)
	if err != nil {
		return fmt.Errorf("failed to get giveaway: %w", err)
	}

	// Get prize information
	prizes := make(map[int]models.Prize)
	for _, winner := range winners {
		if winner.Place <= 0 || winner.Place > len(giveaway.Prizes) {
			return fmt.Errorf("invalid winner place: %d", winner.Place)
		}
		prizePlace := giveaway.Prizes[winner.Place-1]
		prize, err := s.repo.GetPrizeTx(ctx, tx, prizePlace.PrizeID)
		if err != nil {
			return fmt.Errorf("failed to get prize %s: %w", prizePlace.PrizeID, err)
		}
		prizes[winner.Place] = *prize
	}

	// Distribute prizes to winners
	for _, winner := range winners {
		prize, exists := prizes[winner.Place]
		if !exists {
			return fmt.Errorf("prize not found for place %d", winner.Place)
		}

		// Create win record
		winRecord := models.WinRecord{
			GiveawayID: giveawayID,
			UserID:     winner.UserID,
			PrizeID:    prize.ID,
			Place:      winner.Place,
			Status:     models.PrizeStatusPending,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err := s.repo.CreateWinRecordTx(ctx, tx, &winRecord); err != nil {
			return fmt.Errorf("failed to create win record: %w", err)
		}

		// If automatic prize distribution is enabled and prize is internal
		if giveaway.AutoDistribute && prize.IsInternal {
			if err := s.repo.DistributePrizeTx(ctx, tx, giveawayID, winner.UserID, prize.ID); err != nil {
				return fmt.Errorf("failed to distribute prize: %w", err)
			}
		}
	}

	return nil
}

func (s *ExpirationService) notifyWinners(winners []models.Winner, giveawayID string) {
	s.logger.Printf("Starting to notify %d winners for giveaway %s", len(winners), giveawayID)
	for _, winner := range winners {
		if err := s.notifyWinner(winner.UserID, winner.Place, giveawayID); err != nil {
			s.logger.Printf("Error notifying winner %d: %v", winner.UserID, err)
		} else {
			s.logger.Printf("Successfully notified winner %d for giveaway %s", winner.UserID, giveawayID)
		}
	}
}

func (s *ExpirationService) notifyWinner(userID int64, place int, giveawayID string) error {
	s.logger.Printf("Notifying winner %d (place %d) for giveaway %s", userID, place, giveawayID)

	// Get giveaway information
	giveaway, err := s.repo.GetByID(s.ctx, giveawayID)
	if err != nil {
		return fmt.Errorf("failed to get giveaway: %w", err)
	}

	// Get prize information
	if place <= 0 || place > len(giveaway.Prizes) {
		return fmt.Errorf("invalid place: %d", place)
	}
	prizePlace := giveaway.Prizes[place-1]
	prize, err := s.repo.GetPrize(s.ctx, prizePlace.PrizeID)
	if err != nil {
		return fmt.Errorf("failed to get prize: %w", err)
	}

	prizeDetail := models.PrizeDetail{
		Type:        prize.Type,
		Name:        prize.Name,
		Description: prize.Description,
		IsInternal:  prize.IsInternal,
		Status:      string(models.PrizeStatusPending),
	}

	err = s.telegramClient.NotifyWinner(userID, giveaway, place, prizeDetail)
	if err != nil {
		s.logger.Printf("Failed to send notification to winner %d: %v", userID, err)
		return err
	}

	s.logger.Printf("Successfully sent notification to winner %d", userID)
	return nil
}
