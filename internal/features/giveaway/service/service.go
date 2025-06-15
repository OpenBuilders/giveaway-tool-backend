package service

import (
	"context"
	"errors"
	"fmt"
	"giveaway-tool-backend/internal/features/giveaway/models"
	"giveaway-tool-backend/internal/features/giveaway/repository"
	"giveaway-tool-backend/internal/platform/telegram"
	"log"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

var (
	ErrNotFound = errors.New("giveaway not found")
	ErrNotOwner = errors.New("you are not the owner of this giveaway")
)

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
}

type giveawayService struct {
	repo           repository.GiveawayRepository
	telegramClient *telegram.Client
	debug          bool
	redisClient    *redis.Client
}

func NewGiveawayService(repo repository.GiveawayRepository, redisClient *redis.Client, debug bool) GiveawayService {
	return &giveawayService{
		repo:           repo,
		telegramClient: telegram.NewClient(),
		debug:          debug,
		redisClient:    redisClient,
	}
}

func (s *giveawayService) Create(ctx context.Context, userID int64, input *models.GiveawayCreate) (*models.GiveawayResponse, error) {
	if s.debug {
		log.Printf("[DEBUG] Creating giveaway with duration: %d seconds", input.Duration)
	}

	if input.MaxParticipants > 0 && input.WinnersCount > input.MaxParticipants {
		return nil, models.ErrInvalidWinnersCount
	}

	// Проверяем минимальную длительность в зависимости от режима
	minDuration := models.MinDurationRelease
	if s.debug {
		minDuration = models.MinDurationDebug
		log.Printf("[DEBUG] Using debug mode minimum duration: %d seconds", minDuration)
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

		if s.debug {
			log.Printf("[DEBUG] Requirements validation passed")
		}
	}

	// Создаем призы
	for i := range input.Prizes {
		prize := &models.Prize{
			ID:          uuid.New().String(),
			Type:        models.PrizeType(input.Prizes[i].PrizeType),
			Name:        fmt.Sprintf("Prize for place %d", input.Prizes[i].GetPlace()),
			Description: fmt.Sprintf("Prize for place %d in giveaway", input.Prizes[i].GetPlace()),
			IsInternal:  true, // По умолчанию считаем призы внутренними
		}

		if input.Prizes[i].IsAllPlaces() {
			prize.Name = "Prize for all winners"
			prize.Description = "Prize for all winners in giveaway"
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
		if s.debug {
			log.Printf("[DEBUG] Failed to send notification to creator: %v", err)
		}
	}

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
	giveaway, err := s.repo.GetByID(ctx, giveawayID)
	if err != nil {
		return nil, ErrNotFound
	}

	return s.toResponse(ctx, giveaway)
}

func (s *giveawayService) GetByIDWithUser(ctx context.Context, giveawayID string, userID int64) (*models.GiveawayResponse, error) {
	giveaway, err := s.repo.GetByID(ctx, giveawayID)
	if err != nil {
		return nil, ErrNotFound
	}

	return s.toResponseWithUser(ctx, giveaway, userID)
}

func (s *giveawayService) GetByCreator(ctx context.Context, userID int64) ([]*models.GiveawayResponse, error) {
	if s.debug {
		log.Printf("[DEBUG] Getting giveaways for user %d", userID)
	}

	giveaways, err := s.repo.GetByCreatorAndStatus(ctx, userID, []models.GiveawayStatus{models.GiveawayStatusActive, models.GiveawayStatusPending})
	if err != nil {
		return nil, fmt.Errorf("failed to get giveaways: %w", err)
	}

	if s.debug {
		log.Printf("[DEBUG] Found %d giveaways for user %d", len(giveaways), userID)
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
	if s.debug {
		log.Printf("[DEBUG] User %d joining giveaway %s", userID, giveawayID)
	}

	// Получаем информацию о розыгрыше
	giveaway, err := s.repo.GetByID(ctx, giveawayID)
	if err != nil {
		return fmt.Errorf("failed to get giveaway: %w", err)
	}

	// Проверяем статус розыгрыша
	if giveaway.Status != models.GiveawayStatusActive {
		return fmt.Errorf("giveaway is not active")
	}

	// Проверяем, не является ли пользователь создателем
	if giveaway.CreatorID == userID {
		return fmt.Errorf("creator cannot participate in their own giveaway")
	}

	// Проверяем, не превышено ли максимальное количество участников
	if giveaway.MaxParticipants > 0 {
		count, err := s.repo.GetParticipantsCount(ctx, giveawayID)
		if err != nil {
			return fmt.Errorf("failed to get participants count: %w", err)
		}
		if int(count) >= giveaway.MaxParticipants {
			return models.ErrMaxParticipantsReached
		}
	}

	// Проверяем требования для участия
	if giveaway.Requirements != nil && len(giveaway.Requirements) > 0 {
		if s.debug {
			log.Printf("[DEBUG] Checking requirements for user %d in giveaway %s", userID, giveawayID)
		}

		// Используем новый метод проверки требований
		results, err := s.CheckRequirements(ctx, userID, giveawayID)
		if err != nil {
			return fmt.Errorf("failed to check requirements: %w", err)
		}

		// Если не все требования выполнены и нет пропущенных проверок из-за RPS
		if !results.AllMet {
			// Проверяем, есть ли пропущенные проверки
			hasSkipped := false
			for _, result := range results.Results {
				if result.Status == models.RequirementStatusSkipped {
					hasSkipped = true
					break
				}
			}

			// Если нет пропущенных проверок, значит требования точно не выполнены
			if !hasSkipped {
				return fmt.Errorf("user does not meet giveaway requirements")
			}
		}

		if s.debug {
			log.Printf("[DEBUG] Requirements check passed for user %d in giveaway %s", userID, giveawayID)
		}
	}

	// Добавляем участника
	if err := s.repo.AddParticipant(ctx, giveawayID, userID); err != nil {
		return fmt.Errorf("failed to add participant: %w", err)
	}

	if s.debug {
		log.Printf("[DEBUG] User %d successfully joined giveaway %s", userID, giveawayID)
	}

	return nil
}

func (s *giveawayService) GetParticipants(ctx context.Context, giveawayID string) ([]int64, error) {
	return s.repo.GetParticipants(ctx, giveawayID)
}

func (s *giveawayService) GetPrizeTemplates(ctx context.Context) ([]*models.PrizeTemplate, error) {
	return s.repo.GetPrizeTemplates(ctx)
}

func (s *giveawayService) CreateCustomPrize(ctx context.Context, input *models.CustomPrizeCreate) (*models.Prize, error) {
	prize := &models.Prize{
		ID:          uuid.New().String(),
		Type:        models.PrizeTypeCustom,
		Name:        input.Name,
		Description: input.Description,
		IsInternal:  false,
	}

	if err := s.repo.CreatePrize(ctx, prize); err != nil {
		return nil, err
	}

	return prize, nil
}

func (s *giveawayService) GetWinners(ctx context.Context, userID int64, giveawayID string) ([]models.Winner, error) {
	if s.debug {
		log.Printf("[DEBUG] Getting winners for giveaway %s", giveawayID)
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

	if s.debug {
		log.Printf("[DEBUG] Retrieved %d winners for giveaway %s", len(winners), giveawayID)
	}

	return winners, nil
}

func (s *giveawayService) AddTickets(ctx context.Context, userID int64, giveawayID string, count int64) error {
	if s.debug {
		log.Printf("[DEBUG] Adding %d tickets for user %d in giveaway %s", count, userID, giveawayID)
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

	if s.debug {
		log.Printf("[DEBUG] Successfully added %d tickets for user %d in giveaway %s", count, userID, giveawayID)
	}

	return nil
}

func (s *giveawayService) toResponse(ctx context.Context, giveaway *models.Giveaway) (*models.GiveawayResponse, error) {
	if s.debug {
		log.Printf("[DEBUG] Converting giveaway %s to response", giveaway.ID)
	}

	// Получаем количество участников
	participantsCount, err := s.repo.GetParticipantsCount(ctx, giveaway.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get participants count: %w", err)
	}

	// Группируем призы по их типу и ID
	uniquePrizes := make(map[string]models.PrizePlace)
	for _, prize := range giveaway.Prizes {
		prizeKey := fmt.Sprintf("%s_%s", prize.PrizeType, prize.PrizeID)
		if _, exists := uniquePrizes[prizeKey]; !exists {
			// Если это первый приз такого типа, сохраняем его с place = "all"
			prizeCopy := prize
			prizeCopy.Place = "all"
			uniquePrizes[prizeKey] = prizeCopy
		}
	}

	// Преобразуем map в slice
	prizes := make([]models.PrizePlace, 0, len(uniquePrizes))
	for _, prize := range uniquePrizes {
		prizes = append(prizes, prize)
	}

	response := &models.GiveawayResponse{
		ID:                giveaway.ID,
		CreatorID:         giveaway.CreatorID,
		Title:             giveaway.Title,
		Description:       giveaway.Description,
		StartedAt:         giveaway.StartedAt,
		EndsAt:            giveaway.StartedAt.Add(time.Duration(giveaway.Duration) * time.Second),
		MaxParticipants:   giveaway.MaxParticipants,
		WinnersCount:      giveaway.WinnersCount,
		Status:            giveaway.Status,
		CreatedAt:         giveaway.CreatedAt,
		UpdatedAt:         giveaway.UpdatedAt,
		ParticipantsCount: participantsCount,
		CanEdit:           giveaway.IsEditable(),
		UserRole:          "user", // Default role
		Prizes:            prizes,
		Requirements:      giveaway.Requirements,
		AutoDistribute:    giveaway.AutoDistribute,
		AllowTickets:      giveaway.AllowTickets,
		MsgID:             giveaway.MsgID,
		Sponsors:          giveaway.Sponsors,
	}

	// Получаем победителей для завершенных розыгрышей и в процессе распределения наград
	if giveaway.Status == models.GiveawayStatusCompleted || giveaway.Status == models.GiveawayStatusHistory || giveaway.Status == models.GiveawayStatusProcessing {
		winners, err := s.repo.GetWinners(ctx, giveaway.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get winners: %w", err)
		}
		// Сортируем победителей по месту
		sort.Slice(winners, func(i, j int) bool {
			return winners[i].Place < winners[j].Place
		})
		response.Winners = winners
	}

	if s.debug {
		log.Printf("[DEBUG] Successfully converted giveaway %s to response", giveaway.ID)
	}

	return response, nil
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
	if s.debug {
		log.Printf("[DEBUG] Getting historical giveaways for user %d", userID)
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

	if s.debug {
		log.Printf("[DEBUG] Retrieved %d historical giveaways for user %d", len(responses), userID)
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
	responses := make([]*models.GiveawayDetailedResponse, len(giveaways))
	for i, giveaway := range giveaways {
		response, err := s.toDetailedResponse(ctx, giveaway, userID)
		if err != nil {
			return nil, err
		}
		responses[i] = response
	}
	return responses, nil
}

func (s *giveawayService) toDetailedResponse(ctx context.Context, giveaway *models.Giveaway, userID int64) (*models.GiveawayDetailedResponse, error) {
	participantsCount, err := s.repo.GetParticipantsCount(ctx, giveaway.ID)
	if err != nil {
		return nil, err
	}

	// Получаем информацию о создателе
	creator, err := s.repo.GetCreator(ctx, giveaway.CreatorID)
	if err != nil {
		return nil, err
	}

	// Формируем детальную информацию о призах
	uniquePrizes := make(map[string]models.PrizeDetail)
	for _, prize := range giveaway.Prizes {
		prizeInfo, err := s.repo.GetPrize(ctx, prize.PrizeID)
		if err != nil {
			return nil, err
		}

		// Создаем ключ для уникального приза (комбинация типа и имени)
		prizeKey := string(prizeInfo.Type) + "_" + prizeInfo.Name

		// Если такой приз уже есть, пропускаем
		if _, exists := uniquePrizes[prizeKey]; !exists {
			uniquePrizes[prizeKey] = models.PrizeDetail{
				Type:        prizeInfo.Type,
				Name:        prizeInfo.Name,
				Description: prizeInfo.Description,
				IsInternal:  prizeInfo.IsInternal,
				Status:      s.getPrizeStatus(ctx, giveaway.ID, prize.PrizeID),
			}
		}
	}

	// Преобразуем map в slice
	prizes := make([]models.PrizeDetail, 0, len(uniquePrizes))
	for _, prize := range uniquePrizes {
		prizes = append(prizes, prize)
	}

	// Определяем роль пользователя
	userRole := "viewer"
	if giveaway.CreatorID == userID {
		userRole = "owner"
	} else {
		isParticipant, err := s.repo.IsParticipant(ctx, giveaway.ID, userID)
		if err != nil {
			return nil, err
		}
		if isParticipant {
			userRole = "participant"
		}
	}

	// Получаем информацию о билетах
	userTickets := 0
	totalTickets := 0
	if giveaway.AllowTickets {
		userTickets, err = s.repo.GetUserTickets(ctx, giveaway.ID, userID)
		if err != nil {
			return nil, err
		}
		totalTickets, err = s.repo.GetTotalTickets(ctx, giveaway.ID)
		if err != nil {
			return nil, err
		}
	}

	var winnerDetails []models.WinnerDetail
	if giveaway.Status == models.GiveawayStatusCompleted || giveaway.Status == models.GiveawayStatusHistory || giveaway.Status == models.GiveawayStatusProcessing {
		winners, err := s.repo.GetWinners(ctx, giveaway.ID)
		if err != nil {
			return nil, err
		}
		// Сортируем победителей по месту
		sort.Slice(winners, func(i, j int) bool {
			return winners[i].Place < winners[j].Place
		})
		winnerDetails = make([]models.WinnerDetail, len(winners))
		for i, winner := range winners {
			winnerUser, err := s.repo.GetUser(ctx, winner.UserID)
			if err != nil {
				return nil, err
			}
			prizeInfo, err := s.repo.GetPrize(ctx, giveaway.Prizes[winner.Place-1].PrizeID)
			if err != nil {
				return nil, err
			}
			winnerDetails[i] = models.WinnerDetail{
				UserID:   winner.UserID,
				Username: winnerUser.Username,
				Place:    winner.Place,
				Prize: models.PrizeDetail{
					Type:        prizeInfo.Type,
					Name:        prizeInfo.Name,
					Description: prizeInfo.Description,
					IsInternal:  prizeInfo.IsInternal,
					Status:      s.getPrizeStatus(ctx, giveaway.ID, giveaway.Prizes[winner.Place-1].PrizeID),
				},
				ReceivedAt: s.getPrizeReceivedTime(ctx, giveaway.ID, winner.UserID),
			}
		}
	}

	return &models.GiveawayDetailedResponse{
		ID:                giveaway.ID,
		CreatorID:         giveaway.CreatorID,
		CreatorUsername:   creator.Username,
		Title:             giveaway.Title,
		Description:       giveaway.Description,
		StartedAt:         giveaway.StartedAt,
		EndsAt:            giveaway.StartedAt.Add(time.Duration(giveaway.Duration) * time.Second),
		Duration:          giveaway.Duration,
		MaxParticipants:   giveaway.MaxParticipants,
		ParticipantsCount: participantsCount,
		WinnersCount:      giveaway.WinnersCount,
		Status:            giveaway.Status,
		CreatedAt:         giveaway.CreatedAt,
		UpdatedAt:         giveaway.UpdatedAt,
		Winners:           winnerDetails,
		Prizes:            prizes,
		UserRole:          userRole,
		UserTickets:       userTickets,
		TotalTickets:      totalTickets,
	}, nil
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
	if s.debug {
		log.Printf("[DEBUG] Getting top %d giveaways", limit)
	}

	giveaways, err := s.repo.GetTopGiveaways(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get top giveaways: %w", err)
	}

	responses := make([]*models.GiveawayResponse, len(giveaways))
	for i, giveaway := range giveaways {
		response, err := s.toResponse(ctx, giveaway)
		if err != nil {
			return nil, fmt.Errorf("failed to convert giveaway to response: %w", err)
		}
		responses[i] = response
	}

	if s.debug {
		log.Printf("[DEBUG] Retrieved %d top giveaways", len(responses))
	}

	return responses, nil
}

// GetRequirementTemplates возвращает список доступных шаблонов требований для розыгрышей
func (s *giveawayService) GetRequirementTemplates(ctx context.Context) ([]*models.RequirementTemplate, error) {
	if s.debug {
		log.Printf("[DEBUG] Getting requirement templates from repository")
	}

	// Получаем шаблоны из репозитория
	templates, err := s.repo.GetRequirementTemplates(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get requirement templates: %w", err)
	}

	if s.debug {
		log.Printf("[DEBUG] Retrieved %d requirement templates", len(templates))
	}

	return templates, nil
}

func (s *giveawayService) GetAllCreatedGiveaways(ctx context.Context, userID int64) ([]*models.GiveawayDetailedResponse, error) {
	if s.debug {
		log.Printf("[DEBUG] Getting all giveaways for user %d", userID)
	}

	// Получаем все розыгрыши пользователя (активные, завершенные и исторические)
	giveaways, err := s.repo.GetByCreatorAndStatus(ctx, userID, []models.GiveawayStatus{
		models.GiveawayStatusActive,
		models.GiveawayStatusPending,
		models.GiveawayStatusCompleted,
		models.GiveawayStatusHistory,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get giveaways: %w", err)
	}

	if s.debug {
		log.Printf("[DEBUG] Found %d giveaways for user %d", len(giveaways), userID)
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
		if s.debug {
			log.Printf("[DEBUG] Failed to send notification to creator: %v", err)
		}
	}

	return s.toResponse(ctx, newGiveaway)
}

func (s *giveawayService) CheckRequirements(ctx context.Context, userID int64, giveawayID string) (*models.RequirementsCheckResponse, error) {
	if s.debug {
		log.Printf("[DEBUG] Checking requirements for user %d in giveaway %s", userID, giveawayID)
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
			result.ChatInfo = &models.ChatInfo{
				Title:    chatInfo.Title,
				Username: chatInfo.Username,
				Type:     chatInfo.Type,
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

	if s.debug {
		log.Printf("[DEBUG] Requirements check completed for user %d in giveaway %s, all met: %v", userID, giveawayID, allMet)
	}

	return &models.RequirementsCheckResponse{
		GiveawayID: giveawayID,
		Results:    results,
		AllMet:     allMet,
	}, nil
}
