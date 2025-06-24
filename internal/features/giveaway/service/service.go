package service

import (
	"context"
	"errors"
	"fmt"
	"giveaway-tool-backend/internal/common/cache"
	"giveaway-tool-backend/internal/common/config"
	"giveaway-tool-backend/internal/features/giveaway/mapper"
	"giveaway-tool-backend/internal/features/giveaway/models"
	"giveaway-tool-backend/internal/features/giveaway/repository"
	"giveaway-tool-backend/internal/platform/telegram"
	"log"
	"math/rand"
	"time"

	channelservice "giveaway-tool-backend/internal/features/channel/service"

	"github.com/google/uuid"
)

type giveawayService struct {
	repo           repository.GiveawayRepository
	telegramClient *telegram.Client
	cache          *cache.CacheService
	config         *config.Config
	channelService channelservice.ChannelService
	logger         *log.Logger
}

func NewGiveawayService(
	repo repository.GiveawayRepository,
	cache *cache.CacheService,
	config *config.Config,
	channelService channelservice.ChannelService,
	logger *log.Logger,
) GiveawayService {
	return &giveawayService{
		repo:           repo,
		telegramClient: telegram.NewClient(),
		cache:          cache,
		config:         config,
		channelService: channelService,
		logger:         logger,
	}
}

func (s *giveawayService) Create(ctx context.Context, userID int64, input *models.GiveawayCreate) (*models.GiveawayResponse, error) {
	if s.config.Debug {
		s.logger.Printf("[DEBUG] Creating giveaway with duration: %d seconds", input.Duration)
	}

	if input.MaxParticipants > 0 && input.WinnersCount > input.MaxParticipants {
		return nil, models.ErrInvalidWinnersCount
	}

	// Проверяем минимальную длительность в зависимости от режима
	minDuration := models.MinDurationRelease
	if s.config.Debug {
		minDuration = models.MinDurationDebug
		s.logger.Printf("[DEBUG] Using debug mode minimum duration: %d seconds", minDuration)
	}

	if input.Duration < minDuration {
		return nil, fmt.Errorf("duration must be at least %d seconds", minDuration)
	}

	// Проверяем требования, если они включены
	if input.Requirements != nil && len(input.Requirements) > 0 {
		requirements := &models.Requirements{
			Requirements: input.Requirements,
			Enabled:      true,
		}

		// Валидируем структуру требований
		if err := requirements.Validate(); err != nil {
			return nil, fmt.Errorf("invalid requirements: %v", err)
		}

		// Проверяем доступность бота в указанных чатах
		if errors, err := s.telegramClient.ValidateRequirements(requirements); err != nil {
			return nil, fmt.Errorf("failed to validate requirements: %w", err)
		} else if len(errors) > 0 {
			return nil, fmt.Errorf("requirements validation failed: %v", errors)
		}

		if s.config.Debug {
			s.logger.Printf("[DEBUG] Requirements validation passed")
		}
	}

	// Создаем призы
	for i := range input.Prizes {
		prize := &models.Prize{
			ID:         uuid.New().String(),
			Type:       models.PrizeType(input.Prizes[i].PrizeType),
			Name:       fmt.Sprintf("Prize for place %d", input.Prizes[i].GetPlace()),
			IsInternal: true, // По умолчанию считаем призы внутренними
		}

		if input.Prizes[i].IsAllPlaces() {
			prize.Name = "Prize for all winners"
		}

		if err := s.repo.CreatePrize(ctx, prize); err != nil {
			return nil, fmt.Errorf("failed to create prize: %w", err)
		}

		input.Prizes[i].PrizeID = prize.ID
	}

	// Если указан один приз для всех мест, создаем копии для каждого места
	if len(input.Prizes) == 1 && input.Prizes[0].IsAllPlaces() {
		originalPrize := input.Prizes[0]
		input.Prizes = make([]models.PrizePlace, input.WinnersCount)
		for i := 0; i < input.WinnersCount; i++ {
			input.Prizes[i] = models.PrizePlace{
				Place:     i + 1,
				PrizeID:   originalPrize.PrizeID,
				PrizeType: originalPrize.PrizeType,
				Fields:    originalPrize.Fields,
			}
		}
	}

	// Валидация sponsors
	if len(input.Sponsors) > 3 {
		return nil, fmt.Errorf("maximum 3 sponsors allowed")
	}
	seenIDs := make(map[int64]struct{})
	seenUsernames := make(map[string]struct{})
	for _, sponsor := range input.Sponsors {
		if sponsor.ID != 0 {
			if sponsor.ID < 0 || len(fmt.Sprintf("%d", sponsor.ID)) > 100 {
				return nil, fmt.Errorf("invalid sponsor id: %d", sponsor.ID)
			}
			if _, exists := seenIDs[sponsor.ID]; exists {
				return nil, fmt.Errorf("duplicate sponsor id: %d", sponsor.ID)
			}
			seenIDs[sponsor.ID] = struct{}{}
		}
		if sponsor.Username != "" {
			if len(sponsor.Username) > 100 {
				return nil, fmt.Errorf("sponsor username too long: %s", sponsor.Username)
			}
			if _, exists := seenUsernames[sponsor.Username]; exists {
				return nil, fmt.Errorf("duplicate sponsor username: %s", sponsor.Username)
			}
			seenUsernames[sponsor.Username] = struct{}{}
		}
	}

	giveaway := &models.Giveaway{
		ID:              uuid.New().String(),
		CreatorID:       userID,
		Title:           input.Title,
		Description:     input.Description,
		StartedAt:       time.Now(),
		Duration:        input.Duration,
		MaxParticipants: input.MaxParticipants,
		WinnersCount:    input.WinnersCount,
		Status:          models.GiveawayStatusActive,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		Prizes:          input.Prizes,
		AutoDistribute:  input.AutoDistribute,
		AllowTickets:    input.AllowTickets,
		Requirements:    input.Requirements,
		MsgID:           0,
		Sponsors:        input.Sponsors,
	}

	if err := s.repo.Create(ctx, giveaway); err != nil {
		return nil, err
	}

	// Отправляем уведомление создателю
	if err := s.telegramClient.NotifyCreator(userID, giveaway); err != nil {
		if s.config.Debug {
			s.logger.Printf("[DEBUG] Failed to send notification to creator: %v", err)
		}
	}

	// Инвалидируем кэш
	s.cache.InvalidateGiveawayCache(ctx, giveaway.ID)

	return s.toResponse(ctx, giveaway)
}

func (s *giveawayService) Update(ctx context.Context, userID int64, giveawayID string, input *models.GiveawayUpdate) (*models.GiveawayResponse, error) {
	giveaway, err := s.repo.GetByID(ctx, giveawayID)
	if err != nil {
		return nil, ErrNotFound
	}

	if giveaway.CreatorID != userID {
		return nil, ErrNotOwner
	}

	if !giveaway.IsEditable() {
		return nil, models.ErrGiveawayNotEditable
	}

	if input.Title != nil {
		giveaway.Title = *input.Title
	}
	if input.Description != nil {
		giveaway.Description = *input.Description
	}
	if len(input.Prizes) > 0 {
		giveaway.Prizes = input.Prizes
	}
	// Валидация sponsors при обновлении
	if input.Sponsors != nil && len(input.Sponsors) > 0 {
		if len(input.Sponsors) > 3 {
			return nil, fmt.Errorf("maximum 3 sponsors allowed")
		}
		seenIDs := make(map[int64]struct{})
		seenUsernames := make(map[string]struct{})
		for _, sponsor := range input.Sponsors {
			if sponsor.ID != 0 {
				if sponsor.ID < 0 || len(fmt.Sprintf("%d", sponsor.ID)) > 100 {
					return nil, fmt.Errorf("invalid sponsor id: %d", sponsor.ID)
				}
				if _, exists := seenIDs[sponsor.ID]; exists {
					return nil, fmt.Errorf("duplicate sponsor id: %d", sponsor.ID)
				}
				seenIDs[sponsor.ID] = struct{}{}
			}
			if sponsor.Username != "" {
				if len(sponsor.Username) > 100 {
					return nil, fmt.Errorf("sponsor username too long: %s", sponsor.Username)
				}
				if _, exists := seenUsernames[sponsor.Username]; exists {
					return nil, fmt.Errorf("duplicate sponsor username: %s", sponsor.Username)
				}
				seenUsernames[sponsor.Username] = struct{}{}
			}
		}
		giveaway.Sponsors = input.Sponsors
	}

	giveaway.UpdatedAt = time.Now()

	if err := s.repo.Update(ctx, giveaway); err != nil {
		return nil, err
	}

	return s.toResponse(ctx, giveaway)
}

func (s *giveawayService) Delete(ctx context.Context, userID int64, giveawayID string) error {
	giveaway, err := s.repo.GetByID(ctx, giveawayID)
	if err != nil {
		return ErrNotFound
	}

	if giveaway.CreatorID != userID {
		return ErrNotOwner
	}

	return s.repo.Delete(ctx, giveawayID)
}

func (s *giveawayService) GetByID(ctx context.Context, giveawayID string) (*models.GiveawayResponse, error) {
	var response models.GiveawayResponse
	cacheKey := fmt.Sprintf("giveaway:%s", giveawayID)

	err := s.cache.GetOrSet(ctx, cacheKey, &response, s.config.Cache.GiveawayTTL, func() (interface{}, error) {
		giveaway, err := s.repo.GetByID(ctx, giveawayID)
		if err != nil {
			return nil, err
		}
		return s.toResponse(ctx, giveaway)
	})

	if err != nil {
		return nil, err
	}

	return &response, nil
}

func (s *giveawayService) GetByIDWithUser(ctx context.Context, giveawayID string, userID int64) (*models.GiveawayResponse, error) {
	giveaway, err := s.repo.GetByID(ctx, giveawayID)
	if err != nil {
		return nil, ErrNotFound
	}

	return s.toResponseWithUser(ctx, giveaway, userID)
}

func (s *giveawayService) GetByCreator(ctx context.Context, userID int64) ([]*models.GiveawayResponse, error) {
	if s.config.Debug {
		s.logger.Printf("[DEBUG] Getting giveaways for user %d", userID)
	}

	giveaways, err := s.repo.GetByCreatorAndStatus(ctx, userID, []models.GiveawayStatus{models.GiveawayStatusActive, models.GiveawayStatusPending})
	if err != nil {
		return nil, fmt.Errorf("failed to get giveaways: %w", err)
	}

	if s.config.Debug {
		s.logger.Printf("[DEBUG] Found %d giveaways for user %d", len(giveaways), userID)
	}

	responses := make([]*models.GiveawayResponse, len(giveaways))
	for i, giveaway := range giveaways {
		response, err := s.toResponse(ctx, giveaway)
		if err != nil {
			return nil, fmt.Errorf("failed to convert giveaway to response: %w", err)
		}
		responses[i] = response
	}

	return responses, nil
}

func (s *giveawayService) Join(ctx context.Context, userID int64, giveawayID string) error {
	giveaway, err := s.repo.GetByID(ctx, giveawayID)
	if err != nil {
		return ErrNotFound
	}

	// Проверяем, что гив активен или в статусе custom (но не завершен)
	if giveaway.Status != models.GiveawayStatusActive && giveaway.Status != models.GiveawayStatusCustom {
		return fmt.Errorf("giveaway is not active")
	}

	// Если гив в статусе custom, запрещаем присоединение
	if giveaway.Status == models.GiveawayStatusCustom {
		return fmt.Errorf("giveaway is waiting for custom requirements verification, no new participants allowed")
	}

	// Проверяем, не является ли пользователь создателем
	if giveaway.CreatorID == userID {
		return fmt.Errorf("creator cannot join their own giveaway")
	}

	// Проверяем максимальное количество участников
	if giveaway.MaxParticipants > 0 {
		participantsCount, err := s.repo.GetParticipantsCount(ctx, giveawayID)
		if err != nil {
			return fmt.Errorf("failed to get participants count: %w", err)
		}
		if participantsCount >= int64(giveaway.MaxParticipants) {
			return fmt.Errorf("giveaway is full")
		}
	}

	// Проверяем, не участвует ли уже пользователь
	isParticipant, err := s.repo.IsParticipant(ctx, giveawayID, userID)
	if err != nil {
		return fmt.Errorf("failed to check participation: %w", err)
	}
	if isParticipant {
		return fmt.Errorf("user is already a participant")
	}

	// Проверяем требования, если они есть
	if giveaway.Requirements != nil && len(giveaway.Requirements) > 0 {
		// Проверяем только не-кастомные требования (Custom не мешают присоединению)
		nonCustomReqs := make([]models.Requirement, 0)
		for _, req := range giveaway.Requirements {
			if !req.IsCustom() {
				nonCustomReqs = append(nonCustomReqs, req)
			}
		}

		if len(nonCustomReqs) > 0 {
			requirements := &models.Requirements{
				Requirements: nonCustomReqs,
				Enabled:      true,
			}

			allMet, err := s.telegramClient.CheckRequirements(ctx, userID, requirements)
			if err != nil {
				return fmt.Errorf("failed to check requirements: %w", err)
			}
			if !allMet {
				return fmt.Errorf("user does not meet all requirements")
			}
		}
		// Если есть только Custom требования, пропускаем проверку - они не мешают присоединению
	}

	// Добавляем участника
	if err := s.repo.AddParticipant(ctx, giveawayID, userID); err != nil {
		return fmt.Errorf("failed to add participant: %w", err)
	}

	if s.config.Debug {
		s.logger.Printf("[DEBUG] User %d joined giveaway %s", userID, giveawayID)
	}

	return nil
}

func (s *giveawayService) GetParticipants(ctx context.Context, giveawayID string) ([]int64, error) {
	return s.repo.GetParticipants(ctx, giveawayID)
}

func (s *giveawayService) GetPrizeTemplates(ctx context.Context) ([]*models.PrizeTemplate, error) {
	var templates []*models.PrizeTemplate
	cacheKey := "prize_templates"

	err := s.cache.GetOrSet(ctx, cacheKey, &templates, s.config.Cache.PrizeTemplatesTTL, func() (interface{}, error) {
		return s.repo.GetPrizeTemplates(ctx)
	})

	if err != nil {
		return nil, err
	}

	return templates, nil
}

func (s *giveawayService) CreateCustomPrize(ctx context.Context, input *models.CustomPrizeCreate) (*models.Prize, error) {
	prize := &models.Prize{
		ID:         uuid.New().String(),
		Type:       models.PrizeTypeCustom,
		Name:       input.Name,
		IsInternal: false,
	}

	if err := s.repo.CreatePrize(ctx, prize); err != nil {
		return nil, err
	}

	return prize, nil
}

func (s *giveawayService) GetWinners(ctx context.Context, userID int64, giveawayID string) ([]models.Winner, error) {
	if s.config.Debug {
		s.logger.Printf("[DEBUG] Getting winners for giveaway %s", giveawayID)
	}

	// Получаем информацию о розыгрыше
	giveaway, err := s.repo.GetByID(ctx, giveawayID)
	if err != nil {
		return nil, fmt.Errorf("failed to get giveaway: %w", err)
	}

	// Проверяем права доступа
	if giveaway.CreatorID != userID {
		return nil, fmt.Errorf("only creator can view winners")
	}

	// Получаем победителей
	winners, err := s.repo.GetWinners(ctx, giveawayID)
	if err != nil {
		return nil, fmt.Errorf("failed to get winners: %w", err)
	}

	if s.config.Debug {
		s.logger.Printf("[DEBUG] Retrieved %d winners for giveaway %s", len(winners), giveawayID)
	}

	return winners, nil
}

func (s *giveawayService) AddTickets(ctx context.Context, userID int64, giveawayID string, count int64) error {
	if s.config.Debug {
		s.logger.Printf("[DEBUG] Adding %d tickets for user %d in giveaway %s", count, userID, giveawayID)
	}

	// Получаем информацию о розыгрыше
	giveaway, err := s.repo.GetByID(ctx, giveawayID)
	if err != nil {
		return fmt.Errorf("failed to get giveaway: %w", err)
	}

	// Проверяем, разрешены ли билеты
	if !giveaway.AllowTickets {
		return fmt.Errorf("tickets are not allowed in this giveaway")
	}

	// Проверяем, является ли пользователь участником
	isParticipant, err := s.repo.IsParticipant(ctx, giveawayID, userID)
	if err != nil {
		return fmt.Errorf("failed to check participant status: %w", err)
	}
	if !isParticipant {
		return fmt.Errorf("user is not a participant")
	}

	// Добавляем билеты
	if err := s.repo.AddTickets(ctx, giveawayID, userID, count); err != nil {
		return fmt.Errorf("failed to add tickets: %w", err)
	}

	if s.config.Debug {
		s.logger.Printf("[DEBUG] Successfully added %d tickets for user %d in giveaway %s", count, userID, giveawayID)
	}

	return nil
}

func (s *giveawayService) toResponse(ctx context.Context, giveaway *models.Giveaway) (*models.GiveawayResponse, error) {
	return mapper.ToGiveawayResponse(
		ctx,
		giveaway,
		s.repo,
		s.channelService,
		s.config.Debug,
		s.logger.Printf,
	)
}

func (s *giveawayService) toResponseWithUser(ctx context.Context, giveaway *models.Giveaway, userID int64) (*models.GiveawayResponse, error) {
	response, err := s.toResponse(ctx, giveaway)
	if err != nil {
		return nil, err
	}

	if giveaway.CreatorID == userID {
		response.UserRole = "owner"
	} else {
		isParticipant, err := s.repo.IsParticipant(ctx, giveaway.ID, userID)
		if err != nil {
			return nil, err
		}
		if isParticipant {
			response.UserRole = "participant"
		}
	}

	return response, nil
}

// GetHistoricalGiveaways возвращает список исторических розыгрышей пользователя
func (s *giveawayService) GetHistoricalGiveaways(ctx context.Context, userID int64) ([]*models.GiveawayResponse, error) {
	if s.config.Debug {
		s.logger.Printf("[DEBUG] Getting historical giveaways for user %d", userID)
	}

	// Получаем исторические розыгрыши
	giveaways, err := s.repo.GetByCreatorAndStatus(ctx, userID, []models.GiveawayStatus{models.GiveawayStatusHistory})
	if err != nil {
		return nil, fmt.Errorf("failed to get historical giveaways: %w", err)
	}

	// Преобразуем в ответы
	responses := make([]*models.GiveawayResponse, len(giveaways))
	for i, giveaway := range giveaways {
		response, err := s.toResponse(ctx, giveaway)
		if err != nil {
			return nil, fmt.Errorf("failed to convert giveaway to response: %w", err)
		}
		responses[i] = response
	}

	if s.config.Debug {
		s.logger.Printf("[DEBUG] Retrieved %d historical giveaways for user %d", len(responses), userID)
	}

	return responses, nil
}

// MoveToHistory перемещает розыгрыш в историю
func (s *giveawayService) MoveToHistory(ctx context.Context, userID int64, giveawayID string) error {
	giveaway, err := s.repo.GetByID(ctx, giveawayID)
	if err != nil {
		return ErrNotFound
	}

	if giveaway.CreatorID != userID {
		return ErrNotOwner
	}

	if giveaway.Status != models.GiveawayStatusCompleted {
		return errors.New("only completed giveaways can be moved to history")
	}

	return s.repo.MoveToHistory(ctx, giveawayID)
}

func (s *giveawayService) GetCreatedGiveaways(ctx context.Context, userID int64) ([]*models.GiveawayDetailedResponse, error) {
	giveaways, err := s.repo.GetByCreatorAndStatus(ctx, userID, []models.GiveawayStatus{models.GiveawayStatusActive, models.GiveawayStatusPending})
	if err != nil {
		return nil, err
	}
	return s.toDetailedResponses(ctx, giveaways, userID)
}

func (s *giveawayService) GetParticipatedGiveaways(ctx context.Context, userID int64) ([]*models.GiveawayDetailedResponse, error) {
	giveaways, err := s.repo.GetByParticipantAndStatus(ctx, userID, []models.GiveawayStatus{models.GiveawayStatusActive, models.GiveawayStatusPending})
	if err != nil {
		return nil, err
	}
	return s.toDetailedResponses(ctx, giveaways, userID)
}

func (s *giveawayService) GetCreatedGiveawaysHistory(ctx context.Context, userID int64) ([]*models.GiveawayDetailedResponse, error) {
	giveaways, err := s.repo.GetByCreatorAndStatus(ctx, userID, []models.GiveawayStatus{models.GiveawayStatusCompleted, models.GiveawayStatusHistory})
	if err != nil {
		return nil, err
	}
	return s.toDetailedResponses(ctx, giveaways, userID)
}

func (s *giveawayService) GetParticipationHistory(ctx context.Context, userID int64) ([]*models.GiveawayDetailedResponse, error) {
	giveaways, err := s.repo.GetByParticipantAndStatus(ctx, userID, []models.GiveawayStatus{models.GiveawayStatusCompleted, models.GiveawayStatusHistory})
	if err != nil {
		return nil, err
	}
	return s.toDetailedResponses(ctx, giveaways, userID)
}

func (s *giveawayService) toDetailedResponses(ctx context.Context, giveaways []*models.Giveaway, userID int64) ([]*models.GiveawayDetailedResponse, error) {
	return mapper.ToDetailedResponses(
		ctx,
		giveaways,
		s.repo,
		userID,
		s.getPrizeStatus,
		s.getPrizeReceivedTime,
		s.logger.Printf,
	)
}

func (s *giveawayService) toDetailedResponse(ctx context.Context, giveaway *models.Giveaway, userID int64) (*models.GiveawayDetailedResponse, error) {
	return mapper.ToDetailedResponse(
		ctx,
		giveaway,
		s.repo,
		userID,
		s.getPrizeStatus,
		s.getPrizeReceivedTime,
		s.logger.Printf,
	)
}

func (s *giveawayService) getPrizeStatus(ctx context.Context, giveawayID, prizeID string) string {
	// Здесь должна быть логика определения статуса приза
	// Например, проверка был ли приз выдан, отменен и т.д.
	return "pending" // Временная заглушка
}

func (s *giveawayService) getPrizeReceivedTime(ctx context.Context, giveawayID string, userID int64) time.Time {
	// Здесь должна быть логика получения времени выдачи приза
	return time.Time{} // Временная заглушка
}

// GetTopGiveaways returns top giveaways by participants count
func (s *giveawayService) GetTopGiveaways(ctx context.Context, limit int) ([]*models.GiveawayResponse, error) {
	if s.config.Debug {
		s.logger.Printf("[DEBUG] Getting top %d giveaways", limit)
	}

	var responses []*models.GiveawayResponse
	cacheKey := fmt.Sprintf("top_giveaways:%d", limit)

	err := s.cache.GetOrSet(ctx, cacheKey, &responses, s.config.Cache.TopGiveawaysTTL, func() (interface{}, error) {
		giveaways, err := s.repo.GetTopGiveaways(ctx, limit)
		if err != nil {
			return nil, err
		}

		responses := make([]*models.GiveawayResponse, len(giveaways))
		for i, giveaway := range giveaways {
			response, err := s.toResponse(ctx, giveaway)
			if err != nil {
				return nil, err
			}
			responses[i] = response
		}
		return responses, nil
	})

	if err != nil {
		return nil, err
	}

	if s.config.Debug {
		s.logger.Printf("[DEBUG] Retrieved %d top giveaways", len(responses))
	}

	return responses, nil
}

// GetRequirementTemplates возвращает список доступных шаблонов требований для розыгрышей
func (s *giveawayService) GetRequirementTemplates(ctx context.Context) ([]*models.RequirementTemplate, error) {
	if s.config.Debug {
		s.logger.Printf("[DEBUG] Getting requirement templates from repository")
	}

	// Получаем шаблоны из репозитория
	templates, err := s.repo.GetRequirementTemplates(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get requirement templates: %w", err)
	}

	if s.config.Debug {
		s.logger.Printf("[DEBUG] Retrieved %d requirement templates", len(templates))
	}

	return templates, nil
}

func (s *giveawayService) GetAllCreatedGiveaways(ctx context.Context, userID int64) ([]*models.GiveawayDetailedResponse, error) {
	if s.config.Debug {
		s.logger.Printf("[DEBUG] Getting all giveaways for user %d", userID)
	}

	// Получаем все розыгрыши пользователя (активные, завершенные и исторические)
	giveaways, err := s.repo.GetByCreatorAndStatus(ctx, userID, []models.GiveawayStatus{
		models.GiveawayStatusActive,
		models.GiveawayStatusPending,
		models.GiveawayStatusProcessing,
		models.GiveawayStatusCustom,
		models.GiveawayStatusCompleted,
		models.GiveawayStatusHistory,
		models.GiveawayStatusCancelled,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get giveaways: %w", err)
	}

	if s.config.Debug {
		s.logger.Printf("[DEBUG] Found %d giveaways for user %d", len(giveaways), userID)
	}

	return s.toDetailedResponses(ctx, giveaways, userID)
}

func (s *giveawayService) CancelGiveaway(ctx context.Context, userID int64, giveawayID string) error {
	giveaway, err := s.repo.GetByID(ctx, giveawayID)
	if err != nil {
		return ErrNotFound
	}

	if giveaway.CreatorID != userID {
		return ErrNotOwner
	}

	if giveaway.Status != models.GiveawayStatusActive {
		return errors.New("only active giveaways can be cancelled")
	}

	return s.repo.CancelGiveaway(ctx, giveawayID)
}

func (s *giveawayService) RecreateGiveaway(ctx context.Context, userID int64, giveawayID string) (*models.GiveawayResponse, error) {
	giveaway, err := s.repo.GetByID(ctx, giveawayID)
	if err != nil {
		return nil, ErrNotFound
	}

	if giveaway.CreatorID != userID {
		return nil, ErrNotOwner
	}

	if giveaway.Status != models.GiveawayStatusCompleted {
		return nil, errors.New("only completed giveaways can be recreated")
	}

	// Создаем новый розыгрыш с теми же параметрами
	newGiveaway := &models.Giveaway{
		ID:              uuid.New().String(),
		CreatorID:       userID,
		Title:           giveaway.Title,
		Description:     giveaway.Description,
		StartedAt:       time.Now(),
		Duration:        giveaway.Duration,
		MaxParticipants: giveaway.MaxParticipants,
		WinnersCount:    giveaway.WinnersCount,
		Status:          models.GiveawayStatusActive,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		Prizes:          giveaway.Prizes,
		AutoDistribute:  giveaway.AutoDistribute,
		AllowTickets:    giveaway.AllowTickets,
		Requirements:    giveaway.Requirements,
		MsgID:           0,
		Sponsors:        giveaway.Sponsors,
	}

	if err := s.repo.Create(ctx, newGiveaway); err != nil {
		return nil, err
	}

	// Отправляем уведомление создателю
	if err := s.telegramClient.NotifyCreator(userID, newGiveaway); err != nil {
		if s.config.Debug {
			s.logger.Printf("[DEBUG] Failed to send notification to creator: %v", err)
		}
	}

	// Инвалидируем кэш
	s.cache.InvalidateGiveawayCache(ctx, newGiveaway.ID)

	return s.toResponse(ctx, newGiveaway)
}

func (s *giveawayService) CheckRequirements(ctx context.Context, userID int64, giveawayID string) (*models.RequirementsCheckResponse, error) {
	if s.config.Debug {
		s.logger.Printf("[DEBUG] Checking requirements for user %d in giveaway %s", userID, giveawayID)
	}

	// Получаем информацию о розыгрыше
	giveaway, err := s.repo.GetByID(ctx, giveawayID)
	if err != nil {
		return nil, fmt.Errorf("failed to get giveaway: %w", err)
	}

	// Если требований нет, возвращаем успешный результат
	if giveaway.Requirements == nil || len(giveaway.Requirements) == 0 {
		return &models.RequirementsCheckResponse{
			GiveawayID: giveawayID,
			Results:    []models.RequirementCheckResult{},
			AllMet:     true,
		}, nil
	}

	// Создаем слайс для результатов проверки
	results := make([]models.RequirementCheckResult, len(giveaway.Requirements))
	allMet := true

	// Проверяем каждое требование
	for i, req := range giveaway.Requirements {
		result := models.RequirementCheckResult{
			Name:     req.Name(),
			Type:     req.Type,
			Username: req.Username,
			Status:   models.RequirementStatusPending,
		}

		// Получаем информацию о чате
		chatInfo, err := s.telegramClient.GetChat(req.Username)
		if err == nil {
			avatarURL := ""
			if chatInfo.Username != "" {
				avatarURL = fmt.Sprintf("https://t.me/i/userpic/160/%s.jpg", chatInfo.Username)
			}
			result.ChatInfo = &models.ChatInfo{
				Title:     chatInfo.Title,
				Username:  chatInfo.Username,
				Type:      chatInfo.Type,
				AvatarURL: avatarURL,
			}
		}

		// Проверяем требование в зависимости от типа
		switch req.Type {
		case models.RequirementTypeSubscription:
			// Проверяем подписку на канал
			isMember, err := s.telegramClient.CheckMembership(ctx, userID, req.Username)
			if err != nil {
				// Если получили ошибку RPS, помечаем как пропущенное
				var rpsErr *telegram.RPSError
				if errors.As(err, &rpsErr) {
					result.Status = models.RequirementStatusSkipped
					result.Error = "Rate limit exceeded, check skipped"
				} else {
					result.Status = models.RequirementStatusError
					result.Error = err.Error()
				}
				allMet = false
			} else {
				if isMember {
					result.Status = models.RequirementStatusSuccess
				} else {
					result.Status = models.RequirementStatusFailed
					allMet = false
				}
			}

		case models.RequirementTypeBoost:
			// Проверяем буст канала
			hasBoost, err := s.telegramClient.CheckBoost(ctx, userID, req.Username)
			if err != nil {
				// Если получили ошибку RPS, помечаем как пропущенное
				var rpsErr *telegram.RPSError
				if errors.As(err, &rpsErr) {
					result.Status = models.RequirementStatusSkipped
					result.Error = "Rate limit exceeded, check skipped"
				} else {
					result.Status = models.RequirementStatusError
					result.Error = err.Error()
				}
				allMet = false
			} else {
				if hasBoost {
					result.Status = models.RequirementStatusSuccess
				} else {
					result.Status = models.RequirementStatusFailed
					allMet = false
				}
			}

		default:
			result.Status = models.RequirementStatusError
			result.Error = "Unknown requirement type"
			allMet = false
		}

		results[i] = result
	}

	if s.config.Debug {
		s.logger.Printf("[DEBUG] Requirements check completed for user %d in giveaway %s, all met: %v", userID, giveawayID, allMet)
	}

	return &models.RequirementsCheckResponse{
		GiveawayID: giveawayID,
		Results:    results,
		AllMet:     allMet,
	}, nil
}

// HasCustomRequirements проверяет, есть ли в гиве кастомные требования
func (s *giveawayService) HasCustomRequirements(ctx context.Context, giveawayID string) (bool, error) {
	giveaway, err := s.repo.GetByID(ctx, giveawayID)
	if err != nil {
		return false, fmt.Errorf("failed to get giveaway: %w", err)
	}

	if giveaway.Requirements == nil || len(giveaway.Requirements) == 0 {
		return false, nil
	}

	for _, req := range giveaway.Requirements {
		if req.IsCustom() {
			return true, nil
		}
	}

	return false, nil
}

// ValidatePreWinnerUsers валидирует пользователей из pre-winner list
func (s *giveawayService) ValidatePreWinnerUsers(ctx context.Context, userID int64, giveawayID string, userIDs []int64) (*models.PreWinnerValidationResponse, error) {
	// Проверяем, что пользователь является создателем гива
	giveaway, err := s.repo.GetByID(ctx, giveawayID)
	if err != nil {
		return nil, fmt.Errorf("failed to get giveaway: %w", err)
	}

	if giveaway.CreatorID != userID {
		return nil, fmt.Errorf("only giveaway creator can validate pre-winner list")
	}

	// Проверяем, что гив завершен
	if giveaway.Status != models.GiveawayStatusCompleted {
		return nil, fmt.Errorf("giveaway must be completed to process pre-winner list")
	}

	// Получаем всех участников гива
	participants, err := s.repo.GetParticipants(ctx, giveawayID)
	if err != nil {
		return nil, fmt.Errorf("failed to get participants: %w", err)
	}

	// Создаем map для быстрого поиска участников
	participantsMap := make(map[int64]bool)
	for _, participantID := range participants {
		participantsMap[participantID] = true
	}

	results := make([]models.PreWinnerValidationResult, len(userIDs))
	validCount := 0

	for i, userID := range userIDs {
		result := models.PreWinnerValidationResult{
			UserID: userID,
		}

		// Проверяем, является ли пользователь участником
		if !participantsMap[userID] {
			result.Error = "User is not a participant in this giveaway"
			results[i] = result
			continue
		}
		result.IsParticipant = true

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
							result.Error = fmt.Sprintf("Failed to check subscription: %v", err)
							allMet = false
							break
						}
						if !isMember {
							result.Error = "User does not meet subscription requirement"
							allMet = false
							break
						}

					case models.RequirementTypeBoost:
						hasBoost, err := s.telegramClient.CheckBoost(ctx, userID, req.Username)
						if err != nil {
							result.Error = fmt.Sprintf("Failed to check boost: %v", err)
							allMet = false
							break
						}
						if !hasBoost {
							result.Error = "User does not meet boost requirement"
							allMet = false
							break
						}
					}
				}
				result.MeetsRequirements = allMet
			} else {
				// Если нет не-кастомных требований, считаем что все требования выполнены
				result.MeetsRequirements = true
			}
		} else {
			// Если нет требований вообще, считаем что все выполнено
			result.MeetsRequirements = true
		}

		if result.MeetsRequirements {
			validCount++
		}

		results[i] = result
	}

	return &models.PreWinnerValidationResponse{
		GiveawayID: giveawayID,
		Results:    results,
		ValidCount: validCount,
		TotalCount: len(userIDs),
	}, nil
}

// ProcessPreWinnerList обрабатывает загрузку файла с pre-winner list
func (s *giveawayService) ProcessPreWinnerList(ctx context.Context, userID int64, giveawayID string, userIDs []int64) (*models.PreWinnerListResponse, error) {
	// Проверяем, что пользователь является создателем гива
	giveaway, err := s.repo.GetByID(ctx, giveawayID)
	if err != nil {
		return nil, fmt.Errorf("failed to get giveaway: %w", err)
	}

	if giveaway.CreatorID != userID {
		return nil, fmt.Errorf("only giveaway creator can upload pre-winner list")
	}

	// Проверяем, что гив в статусе custom
	if giveaway.Status != models.GiveawayStatusCustom {
		return nil, fmt.Errorf("giveaway must be in custom status to upload pre-winner list")
	}

	// Валидируем пользователей
	validation, err := s.ValidatePreWinnerUsers(ctx, userID, giveawayID, userIDs)
	if err != nil {
		return nil, err
	}

	// Собираем валидных пользователей
	var validUsers []models.PreWinnerUser
	var invalidUsers []int64

	for _, result := range validation.Results {
		if result.IsParticipant && result.MeetsRequirements {
			// Получаем информацию о пользователе
			user, err := s.getUserInfo(ctx, result.UserID)
			if err != nil {
				// Если не удалось получить информацию, создаем базовую
				validUsers = append(validUsers, models.PreWinnerUser{
					UserID:    result.UserID,
					Username:  fmt.Sprintf("user_%d", result.UserID),
					AvatarURL: "",
				})
			} else {
				validUsers = append(validUsers, *user)
			}
		} else {
			invalidUsers = append(invalidUsers, result.UserID)
		}
	}

	// Сохраняем pre-winner list
	preWinnerList := &models.PreWinnerListStored{
		GiveawayID: giveawayID,
		UserIDs:    make([]int64, len(validUsers)),
		Users:      validUsers,
		CreatedAt:  time.Now().Unix(),
	}

	// Извлекаем только ID для сохранения
	for i, user := range validUsers {
		preWinnerList.UserIDs[i] = user.UserID
	}

	// Удаляем старые данные перед сохранением новых (перезагрузка файла)
	s.repo.DeletePreWinnerList(ctx, giveawayID)

	if err := s.repo.SavePreWinnerList(ctx, giveawayID, preWinnerList); err != nil {
		return nil, fmt.Errorf("failed to save pre-winner list: %w", err)
	}

	response := &models.PreWinnerListResponse{
		GiveawayID:    giveawayID,
		TotalUploaded: len(userIDs),
		ValidUsers:    validUsers,
		InvalidUsers:  invalidUsers,
		Message:       fmt.Sprintf("Pre-winner list uploaded successfully. %d valid users, %d invalid users", len(validUsers), len(invalidUsers)),
	}

	return response, nil
}

// CompleteGiveawayWithCustomRequirements завершает гив с кастомными требованиями
func (s *giveawayService) CompleteGiveawayWithCustomRequirements(ctx context.Context, userID int64, giveawayID string) (*models.CompleteWithCustomResponse, error) {
	// Проверяем, что пользователь является создателем гива
	giveaway, err := s.repo.GetByID(ctx, giveawayID)
	if err != nil {
		return nil, fmt.Errorf("failed to get giveaway: %w", err)
	}

	if giveaway.CreatorID != userID {
		return nil, fmt.Errorf("only giveaway creator can complete giveaway with custom requirements")
	}

	// Проверяем, что гив в статусе custom
	if giveaway.Status != models.GiveawayStatusCustom {
		return nil, fmt.Errorf("giveaway must be in custom status to complete with custom requirements")
	}

	// Проверяем, что гив завершен по времени
	if !giveaway.HasEnded() {
		return nil, fmt.Errorf("giveaway has not ended yet")
	}

	// Получаем pre-winner list
	preWinnerList, err := s.repo.GetPreWinnerList(ctx, giveawayID)
	if err != nil {
		return nil, fmt.Errorf("pre-winner list not found: %w", err)
	}

	// Выбираем победителей
	var winners []models.PreWinnerUser

	if len(preWinnerList.Users) >= giveaway.WinnersCount {
		// Custom участников достаточно - выбираем случайным образом из них
		winners = s.selectWinnersFromList(preWinnerList.Users, giveaway.WinnersCount)
	} else {
		// Custom участников недостаточно - сначала берем всех Custom, потом дотягиваем остальных

		// Получаем всех участников гива
		allParticipants, err := s.repo.GetParticipants(ctx, giveawayID)
		if err != nil {
			return nil, fmt.Errorf("failed to get all participants: %w", err)
		}

		// Создаем map для быстрого поиска Custom участников
		customUsersMap := make(map[int64]bool)
		for _, user := range preWinnerList.Users {
			customUsersMap[user.UserID] = true
		}

		// Собираем остальных участников (не Custom, но выполнивших не-кастомные требования)
		var otherValidUsers []models.PreWinnerUser
		for _, participantID := range allParticipants {
			// Пропускаем Custom участников
			if customUsersMap[participantID] {
				continue
			}

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

			// Получаем информацию о пользователе
			user, err := s.getUserInfo(ctx, participantID)
			if err != nil {
				otherValidUsers = append(otherValidUsers, models.PreWinnerUser{
					UserID:    participantID,
					Username:  fmt.Sprintf("user_%d", participantID),
					AvatarURL: "",
				})
			} else {
				otherValidUsers = append(otherValidUsers, *user)
			}
		}

		// Сначала добавляем всех Custom участников
		winners = append(winners, preWinnerList.Users...)

		// Затем дотягиваем остальных случайным образом
		remainingSlots := giveaway.WinnersCount - len(preWinnerList.Users)
		if remainingSlots > 0 && len(otherValidUsers) > 0 {
			additionalWinners := s.selectWinnersFromList(otherValidUsers, remainingSlots)
			winners = append(winners, additionalWinners...)
		}
	}

	// Создаем записи о победах
	for i, winner := range winners {
		winRecord := &models.WinRecord{
			ID:         uuid.New().String(),
			GiveawayID: giveawayID,
			UserID:     winner.UserID,
			Place:      i + 1,
			Status:     models.PrizeStatusPending,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		// Сохраняем запись о победе
		if err := s.repo.CreateWinRecord(ctx, winRecord); err != nil {
			return nil, fmt.Errorf("failed to save win record: %w", err)
		}
	}

	// Обновляем статус гива
	giveaway.Status = models.GiveawayStatusCompleted
	giveaway.UpdatedAt = time.Now()
	if err := s.repo.Update(ctx, giveaway); err != nil {
		return nil, fmt.Errorf("failed to update giveaway status: %w", err)
	}

	// Удаляем pre-winner list
	s.repo.DeletePreWinnerList(ctx, giveawayID)

	response := &models.CompleteWithCustomResponse{
		GiveawayID:   giveawayID,
		WinnersCount: giveaway.WinnersCount,
		Winners:      winners,
		Message:      fmt.Sprintf("Giveaway completed successfully. %d winners selected", len(winners)),
	}

	return response, nil
}

// getUserInfo получает информацию о пользователе
func (s *giveawayService) getUserInfo(ctx context.Context, userID int64) (*models.PreWinnerUser, error) {
	user, err := s.repo.GetUser(ctx, userID)
	if err != nil || user == nil {
		return &models.PreWinnerUser{
			UserID:    userID,
			Username:  fmt.Sprintf("user_%d", userID),
			AvatarURL: "",
		}, nil
	}

	avatarURL := ""
	if user.Username != "" {
		avatarURL = fmt.Sprintf("https://t.me/i/userpic/160/%s.jpg", user.Username)
	}

	return &models.PreWinnerUser{
		UserID:    userID,
		Username:  user.Username,
		AvatarURL: avatarURL,
	}, nil
}

// selectWinnersFromList выбирает победителей из списка пользователей
func (s *giveawayService) selectWinnersFromList(users []models.PreWinnerUser, winnersCount int) []models.PreWinnerUser {
	if len(users) == 0 || winnersCount <= 0 {
		return []models.PreWinnerUser{}
	}

	if winnersCount >= len(users) {
		// Если победителей больше или равно количеству пользователей, возвращаем всех
		return users
	}

	// Перемешиваем список пользователей
	shuffled := make([]models.PreWinnerUser, len(users))
	copy(shuffled, users)

	// Fisher-Yates shuffle
	for i := len(shuffled) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	}

	// Возвращаем первых winnersCount пользователей
	return shuffled[:winnersCount]
}

func (s *giveawayService) GetMyActiveGiveaways(ctx context.Context, userID int64) ([]*models.GiveawayResponse, error) {
	if s.config.Debug {
		s.logger.Printf("[DEBUG] Getting active giveaways for user %d", userID)
	}

	giveaways, err := s.repo.GetByCreatorAndStatus(ctx, userID, []models.GiveawayStatus{models.GiveawayStatusActive, models.GiveawayStatusPending})
	if err != nil {
		return nil, fmt.Errorf("failed to get active giveaways: %w", err)
	}

	if s.config.Debug {
		s.logger.Printf("[DEBUG] Retrieved %d active giveaways for user %d", len(giveaways), userID)
	}

	responses := make([]*models.GiveawayResponse, len(giveaways))
	for i, giveaway := range giveaways {
		response, err := s.toResponse(ctx, giveaway)
		if err != nil {
			return nil, fmt.Errorf("failed to convert giveaway to response: %w", err)
		}
		responses[i] = response
	}

	return responses, nil
}

func (s *giveawayService) GetMyGiveawaysHistory(ctx context.Context, userID int64) ([]*models.GiveawayResponse, error) {
	if s.config.Debug {
		s.logger.Printf("[DEBUG] Getting historical giveaways for user %d", userID)
	}

	giveaways, err := s.repo.GetByCreatorAndStatus(ctx, userID, []models.GiveawayStatus{models.GiveawayStatusHistory})
	if err != nil {
		return nil, fmt.Errorf("failed to get historical giveaways: %w", err)
	}

	if s.config.Debug {
		s.logger.Printf("[DEBUG] Retrieved %d historical giveaways for user %d", len(giveaways), userID)
	}

	responses := make([]*models.GiveawayResponse, len(giveaways))
	for i, giveaway := range giveaways {
		response, err := s.toResponse(ctx, giveaway)
		if err != nil {
			return nil, fmt.Errorf("failed to convert giveaway to response: %w", err)
		}
		responses[i] = response
	}

	return responses, nil
}

func (s *giveawayService) GetMyAwaitingActionGiveaways(ctx context.Context, userID int64) ([]*models.GiveawayResponse, error) {
	if s.config.Debug {
		s.logger.Printf("[DEBUG] Getting awaiting action giveaways for user %d", userID)
	}

	giveaways, err := s.repo.GetByCreatorAndStatus(ctx, userID, []models.GiveawayStatus{models.GiveawayStatusCustom})
	if err != nil {
		return nil, fmt.Errorf("failed to get awaiting action giveaways: %w", err)
	}

	if s.config.Debug {
		s.logger.Printf("[DEBUG] Retrieved %d awaiting action giveaways for user %d", len(giveaways), userID)
	}

	responses := make([]*models.GiveawayResponse, len(giveaways))
	for i, giveaway := range giveaways {
		response, err := s.toResponse(ctx, giveaway)
		if err != nil {
			return nil, fmt.Errorf("failed to convert giveaway to response: %w", err)
		}
		responses[i] = response
	}

	return responses, nil
}
