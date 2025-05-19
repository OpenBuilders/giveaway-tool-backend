package service

import (
	"context"
	"fmt"
	"giveaway-tool-backend/internal/features/giveaway/models"
	"giveaway-tool-backend/internal/features/giveaway/repository"
	"log"
	"os"
	"sync"
	"time"
)

const (
	queueCheckInterval = 30 * time.Second
	maxConcurrent      = 5
)

// QueueService управляет очередью обработки розыгрышей
type QueueService struct {
	ctx            context.Context
	repo           repository.GiveawayRepository
	expirationSvc  *ExpirationService
	processingLock sync.Mutex
	processing     map[string]bool
	logger         *log.Logger
}

func NewQueueService(ctx context.Context, repo repository.GiveawayRepository, expirationSvc *ExpirationService) *QueueService {
	return &QueueService{
		ctx:           ctx,
		repo:          repo,
		expirationSvc: expirationSvc,
		processing:    make(map[string]bool),
		logger:        log.New(os.Stdout, "[QueueService] ", log.LstdFlags),
	}
}

// Start запускает обработку очереди
func (s *QueueService) Start() {
	s.logger.Printf("Starting queue service")
	go s.processQueue()
}

// Stop останавливает обработку очереди
func (s *QueueService) Stop() {
	s.logger.Printf("Stopping queue service")
}

// processQueue обрабатывает очередь розыгрышей
func (s *QueueService) processQueue() {
	ticker := time.NewTicker(queueCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			s.logger.Printf("Queue service stopped")
			return
		case <-ticker.C:
			s.processPendingGiveaways()
		}
	}
}

// processPendingGiveaways обрабатывает ожидающие розыгрыши
func (s *QueueService) processPendingGiveaways() {
	pending, err := s.repo.GetPendingGiveaways(s.ctx)
	if err != nil {
		s.logger.Printf("Error getting pending giveaways: %v", err)
		return
	}

	if len(pending) == 0 {
		return
	}

	s.logger.Printf("Found %d pending giveaways", len(pending))

	// Создаем пул горутин для параллельной обработки
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

	for _, giveaway := range pending {
		giveawayID := giveaway.ID // Сохраняем ID в отдельную переменную

		// Проверяем, не обрабатывается ли уже этот розыгрыш
		s.processingLock.Lock()
		if s.processing[giveawayID] {
			s.processingLock.Unlock()
			continue
		}
		s.processing[giveawayID] = true
		s.processingLock.Unlock()

		wg.Add(1)
		go func(g *models.Giveaway) {
			defer wg.Done()
			defer func() {
				s.processingLock.Lock()
				delete(s.processing, g.ID)
				s.processingLock.Unlock()
			}()

			sem <- struct{}{}        // Получаем слот в семафоре
			defer func() { <-sem }() // Освобождаем слот

			if err := s.processGiveaway(g); err != nil {
				s.logger.Printf("Error processing giveaway %s: %v", g.ID, err)
			}
		}(giveaway)
	}

	wg.Wait()
}

// processGiveaway обрабатывает один розыгрыш
func (s *QueueService) processGiveaway(giveaway *models.Giveaway) error {
	s.logger.Printf("Processing giveaway %s", giveaway.ID)

	// Проверяем статус розыгрыша
	if giveaway.Status != models.GiveawayStatusPending {
		s.logger.Printf("Giveaway %s is not in pending status (current: %s)", giveaway.ID, giveaway.Status)
		return nil
	}

	// Обрабатываем розыгрыш через ExpirationService
	if err := s.expirationSvc.ProcessExpiredGiveaways(); err != nil {
		return fmt.Errorf("failed to process giveaway: %w", err)
	}

	// Удаляем из очереди
	if err := s.repo.AddToHistory(s.ctx, giveaway.ID); err != nil {
		return fmt.Errorf("failed to add to history: %w", err)
	}

	s.logger.Printf("Successfully processed giveaway %s", giveaway.ID)
	return nil
}

// AddToQueue добавляет розыгрыш в очередь
func (s *QueueService) AddToQueue(giveawayID string) error {
	s.logger.Printf("Adding giveaway %s to queue", giveawayID)
	return s.repo.AddToPending(s.ctx, giveawayID)
}

// RemoveFromQueue удаляет розыгрыш из очереди
func (s *QueueService) RemoveFromQueue(giveawayID string) error {
	s.logger.Printf("Removing giveaway %s from queue", giveawayID)
	return s.repo.AddToHistory(s.ctx, giveawayID)
}
