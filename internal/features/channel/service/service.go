package service

import (
	"context"
	"database/sql"
	"fmt"
	"giveaway-tool-backend/internal/common/cache"
	"giveaway-tool-backend/internal/common/errors"
	"giveaway-tool-backend/internal/common/validation"
	"giveaway-tool-backend/internal/features/channel/models"
	"giveaway-tool-backend/internal/features/channel/repository"
	"giveaway-tool-backend/internal/platform/telegram"
	"time"

	"go.uber.org/zap"
)

type channelService struct {
	repo           repository.ChannelRepository
	telegramClient *telegram.Client
	cache          *cache.CacheService
	debug          bool
	logger         *zap.Logger
}

func NewChannelService(repo repository.ChannelRepository, cacheService *cache.CacheService, debug bool, logger *zap.Logger) ChannelService {
	return &channelService{
		repo:           repo,
		telegramClient: telegram.NewClient(),
		cache:          cacheService,
		debug:          debug,
		logger:         logger,
	}
}

// GetChannelAvatar получает CDN ссылку на аватар канала
func (s *channelService) GetChannelAvatar(ctx context.Context, channelID int64) (string, error) {
	// Валидация входных данных
	if err := validation.ValidatePositiveInt(channelID, "channel ID"); err != nil {
		s.logger.Warn("Invalid channel ID provided",
			zap.Int64("channel_id", channelID),
			zap.Error(err),
		)
		return "", errors.NewValidationError("channel_id", "Channel ID must be positive").
			WithDetail("provided_value", channelID)
	}

	if s.debug {
		s.logger.Debug("Getting channel avatar for channel", zap.Int64("channel_id", channelID))
	}

	// Пытаемся получить из кэша
	var avatarURL string
	cacheKey := fmt.Sprintf("channel_avatar:%d", channelID)

	err := s.cache.GetOrSet(ctx, cacheKey, &avatarURL, 30*time.Minute, func() (interface{}, error) {
		channelInfo, err := s.repo.GetChannelInfo(ctx, channelID)
		if err != nil {
			return "", err
		}
		return channelInfo.AvatarURL, nil
	})

	if err != nil {
		s.logger.Error("Failed to get channel avatar",
			zap.Int64("channel_id", channelID),
			zap.Error(err),
		)
		return "", errors.NewDatabaseError("get channel avatar", err).
			WithDetail("channel_id", channelID)
	}

	if s.debug {
		s.logger.Debug("Successfully retrieved avatar URL for channel", zap.Int64("channel_id", channelID))
	}

	return avatarURL, nil
}

// GetUserChannels получает список каналов пользователя
func (s *channelService) GetUserChannels(ctx context.Context, userID int64) ([]int64, error) {
	// Валидация входных данных
	if err := validation.ValidatePositiveInt(userID, "user ID"); err != nil {
		s.logger.Warn("Invalid user ID provided",
			zap.Int64("user_id", userID),
			zap.Error(err),
		)
		return nil, errors.NewValidationError("user_id", "User ID must be positive").
			WithDetail("provided_value", userID)
	}

	if s.debug {
		s.logger.Debug("Getting channels for user", zap.Int64("user_id", userID))
	}

	// В новой архитектуре каналы пользователя хранятся в таблице giveaway_sponsors
	// Это упрощенная реализация - в реальности нужно получать каналы из гивов пользователя
	var channelIDs []int64
	cacheKey := fmt.Sprintf("user_channels:%d", userID)

	err := s.cache.GetOrSet(ctx, cacheKey, &channelIDs, 15*time.Minute, func() (interface{}, error) {
		// Здесь должна быть логика получения каналов пользователя
		// Пока возвращаем пустой список
		return []int64{}, nil
	})

	if err != nil {
		s.logger.Error("Failed to get user channels",
			zap.Int64("user_id", userID),
			zap.Error(err),
		)
		return nil, errors.NewDatabaseError("get user channels", err).
			WithDetail("user_id", userID)
	}

	if s.debug {
		s.logger.Debug("Successfully retrieved channels for user", zap.Int64("user_id", userID), zap.Int("count", len(channelIDs)))
	}

	return channelIDs, nil
}

// GetChannelTitle получает название канала
func (s *channelService) GetChannelTitle(ctx context.Context, channelID int64) (string, error) {
	// Валидация входных данных
	if err := validation.ValidatePositiveInt(channelID, "channel ID"); err != nil {
		s.logger.Warn("Invalid channel ID provided",
			zap.Int64("channel_id", channelID),
			zap.Error(err),
		)
		return "", errors.NewValidationError("channel_id", "Channel ID must be positive").
			WithDetail("provided_value", channelID)
	}

	if s.debug {
		s.logger.Debug("Getting title for channel", zap.Int64("channel_id", channelID))
	}

	// Пытаемся получить из кэша
	var title string
	cacheKey := fmt.Sprintf("channel_title:%d", channelID)

	err := s.cache.GetOrSet(ctx, cacheKey, &title, 30*time.Minute, func() (interface{}, error) {
		return s.repo.GetChannelTitle(ctx, channelID)
	})

	if err != nil {
		s.logger.Error("Failed to get channel title",
			zap.Int64("channel_id", channelID),
			zap.Error(err),
		)
		return "", errors.NewDatabaseError("get channel title", err).
			WithDetail("channel_id", channelID)
	}

	if s.debug {
		s.logger.Debug("Successfully retrieved title for channel", zap.Int64("channel_id", channelID))
	}

	return title, nil
}

// GetChannelUsername получает username канала
func (s *channelService) GetChannelUsername(ctx context.Context, channelID int64) (string, error) {
	// Валидация входных данных
	if err := validation.ValidatePositiveInt(channelID, "channel ID"); err != nil {
		s.logger.Warn("Invalid channel ID provided",
			zap.Int64("channel_id", channelID),
			zap.Error(err),
		)
		return "", errors.NewValidationError("channel_id", "Channel ID must be positive").
			WithDetail("provided_value", channelID)
	}

	return s.repo.GetChannelUsername(ctx, channelID)
}

// GetPublicChannelInfo получает публичную информацию о канале
func (s *channelService) GetPublicChannelInfo(ctx context.Context, username string) (*telegram.PublicChannelInfo, error) {
	// Валидация входных данных
	if err := validation.ValidateChannelUsername(username); err != nil {
		s.logger.Warn("Invalid channel username provided",
			zap.String("username", username),
			zap.Error(err),
		)
		return nil, errors.NewValidationError("username", "Invalid channel username format").
			WithDetail("provided_value", username)
	}

	pubInfo, err := s.telegramClient.GetPublicChannelInfo(ctx, username, s.repo)
	if err != nil {
		s.logger.Error("Failed to get public channel info",
			zap.String("username", username),
			zap.Error(err),
		)
		return nil, errors.NewDatabaseError("get public channel info", err).
			WithDetail("username", username)
	}

	// Сохраняем информацию в PostgreSQL
	info := &models.ChannelInfo{
		ID:         pubInfo.ID,
		Username:   pubInfo.Username,
		Title:      pubInfo.Title,
		AvatarURL:  pubInfo.AvatarURL,
		ChannelURL: pubInfo.ChannelURL,
		CreatedAt:  time.Now(),
	}

	if err := s.repo.SaveChannelInfo(ctx, info); err != nil {
		s.logger.Warn("Failed to save channel info",
			zap.Int64("channel_id", pubInfo.ID),
			zap.Error(err),
		)
	}

	// Инвалидируем кэш канала
	if err := s.cache.InvalidateChannelCache(ctx, pubInfo.ID); err != nil {
		s.logger.Warn("Failed to invalidate channel cache",
			zap.Int64("channel_id", pubInfo.ID),
			zap.Error(err),
		)
	}

	s.logger.Info("Retrieved public channel info",
		zap.Int64("channel_id", pubInfo.ID),
		zap.String("username", pubInfo.Username),
	)
	return pubInfo, nil
}

func (s *channelService) GetChannelInfoByID(ctx context.Context, channelID int64) (*models.ChannelInfo, error) {
	// Валидация входных данных
	if err := validation.ValidatePositiveInt(channelID, "channel ID"); err != nil {
		s.logger.Warn("Invalid channel ID provided",
			zap.Int64("channel_id", channelID),
			zap.Error(err),
		)
		return nil, errors.NewValidationError("channel_id", "Channel ID must be positive").
			WithDetail("provided_value", channelID)
	}

	// Пытаемся получить из кэша
	var channelInfo models.ChannelInfo
	cacheKey := fmt.Sprintf("channel:%d", channelID)

	err := s.cache.GetOrSet(ctx, cacheKey, &channelInfo, 30*time.Minute, func() (interface{}, error) {
		// Сначала пробуем получить из PostgreSQL
		info, err := s.repo.GetChannelInfoByID(ctx, channelID)
		if err == nil && info != nil {
			return info, nil
		}

		// Если нет в PostgreSQL, получаем username
		username, err := s.repo.GetChannelUsername(ctx, channelID)
		if err != nil {
			return nil, err
		}

		// Получаем полную информацию через Telegram API
		pubInfo, err := s.telegramClient.GetPublicChannelInfo(ctx, username, s.repo)
		if err != nil {
			return nil, err
		}

		// Сохраняем в PostgreSQL
		info = &models.ChannelInfo{
			ID:         pubInfo.ID,
			Username:   pubInfo.Username,
			Title:      pubInfo.Title,
			AvatarURL:  pubInfo.AvatarURL,
			ChannelURL: pubInfo.ChannelURL,
			CreatedAt:  time.Now(),
		}

		if err := s.repo.SaveChannelInfo(ctx, info); err != nil {
			s.logger.Warn("Failed to save channel info",
				zap.Int64("channel_id", pubInfo.ID),
				zap.Error(err),
			)
		}

		return info, nil
	})

	if err != nil {
		s.logger.Error("Failed to get channel info by ID",
			zap.Int64("channel_id", channelID),
			zap.Error(err),
		)
		return nil, errors.NewDatabaseError("get channel info by id", err).
			WithDetail("channel_id", channelID)
	}

	return &channelInfo, nil
}

func (s *channelService) GetChannelInfoByUsername(ctx context.Context, username string) (*models.ChannelInfo, error) {
	// Валидация входных данных
	if err := validation.ValidateChannelUsername(username); err != nil {
		s.logger.Warn("Invalid channel username provided",
			zap.String("username", username),
			zap.Error(err),
		)
		return nil, errors.NewValidationError("username", "Invalid channel username format").
			WithDetail("provided_value", username)
	}

	return s.repo.GetChannelInfoByUsername(ctx, username)
}

// CreateChannel создает новый канал
func (s *channelService) CreateChannel(ctx context.Context, channel *models.Channel) error {
	s.logger.Info("Creating new channel",
		zap.Int64("channel_id", channel.ID),
		zap.String("title", channel.Title),
	)

	// Валидация входных данных
	if err := s.validateChannelData(channel); err != nil {
		s.logger.Warn("Channel validation failed",
			zap.Int64("channel_id", channel.ID),
			zap.Error(err),
		)
		return err
	}

	// Проверяем, существует ли канал
	existingChannel, err := s.repo.GetByID(ctx, channel.ID)
	if err != nil && err != sql.ErrNoRows {
		s.logger.Error("Failed to check existing channel",
			zap.Int64("channel_id", channel.ID),
			zap.Error(err),
		)
		return errors.NewDatabaseError("get channel by id", err).
			WithDetail("channel_id", channel.ID)
	}

	if existingChannel != nil {
		s.logger.Info("Channel already exists",
			zap.Int64("channel_id", channel.ID),
			zap.String("title", existingChannel.Title),
		)
		return errors.NewConflictError("channel", fmt.Sprintf("Channel with ID %d already exists", channel.ID)).
			WithDetail("channel_id", channel.ID).
			WithDetail("existing_title", existingChannel.Title)
	}

	// Создаем канал
	if err := s.repo.Create(ctx, channel); err != nil {
		s.logger.Error("Failed to create channel",
			zap.Int64("channel_id", channel.ID),
			zap.Error(err),
		)
		return errors.NewDatabaseError("create channel", err).
			WithDetail("channel_id", channel.ID)
	}

	s.logger.Info("Channel created successfully",
		zap.Int64("channel_id", channel.ID),
		zap.String("title", channel.Title),
		zap.String("status", channel.Status),
	)

	return nil
}

// GetChannel получает канал по ID
func (s *channelService) GetChannel(ctx context.Context, id int64) (*models.Channel, error) {
	s.logger.Debug("Getting channel by ID", zap.Int64("channel_id", id))

	// Валидация входных данных
	if err := validation.ValidatePositiveInt(id, "channel ID"); err != nil {
		s.logger.Warn("Invalid channel ID provided",
			zap.Int64("channel_id", id),
			zap.Error(err),
		)
		return nil, errors.NewValidationError("channel_id", "Channel ID must be positive").
			WithDetail("provided_value", id)
	}

	channel, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			s.logger.Info("Channel not found", zap.Int64("channel_id", id))
			return nil, errors.NewChannelNotFoundError(id)
		}

		s.logger.Error("Failed to get channel by ID",
			zap.Int64("channel_id", id),
			zap.Error(err),
		)
		return nil, errors.NewDatabaseError("get channel by id", err).
			WithDetail("channel_id", id)
	}

	s.logger.Debug("Channel retrieved successfully",
		zap.Int64("channel_id", id),
		zap.String("title", channel.Title),
	)

	return channel, nil
}

// UpdateChannel обновляет данные канала
func (s *channelService) UpdateChannel(ctx context.Context, channel *models.Channel) error {
	s.logger.Info("Updating channel",
		zap.Int64("channel_id", channel.ID),
		zap.String("title", channel.Title),
	)

	// Валидация входных данных
	if err := s.validateChannelData(channel); err != nil {
		s.logger.Warn("Channel validation failed during update",
			zap.Int64("channel_id", channel.ID),
			zap.Error(err),
		)
		return err
	}

	// Проверяем, существует ли канал
	existingChannel, err := s.repo.GetByID(ctx, channel.ID)
	if err != nil {
		if err == sql.ErrNoRows {
			s.logger.Info("Channel not found for update", zap.Int64("channel_id", channel.ID))
			return errors.NewChannelNotFoundError(channel.ID)
		}

		s.logger.Error("Failed to check existing channel for update",
			zap.Int64("channel_id", channel.ID),
			zap.Error(err),
		)
		return errors.NewDatabaseError("get channel by id for update", err).
			WithDetail("channel_id", channel.ID)
	}

	// Проверяем статус канала
	if existingChannel.Status == models.ChannelStatusBanned {
		s.logger.Warn("Attempted to update banned channel",
			zap.Int64("channel_id", channel.ID),
			zap.String("status", existingChannel.Status),
		)
		return errors.NewForbiddenError("Cannot update banned channel").
			WithDetail("channel_id", channel.ID).
			WithDetail("current_status", existingChannel.Status)
	}

	// Обновляем канал
	if err := s.repo.Update(ctx, channel); err != nil {
		s.logger.Error("Failed to update channel",
			zap.Int64("channel_id", channel.ID),
			zap.Error(err),
		)
		return errors.NewDatabaseError("update channel", err).
			WithDetail("channel_id", channel.ID)
	}

	s.logger.Info("Channel updated successfully",
		zap.Int64("channel_id", channel.ID),
		zap.String("title", channel.Title),
		zap.String("status", channel.Status),
	)

	return nil
}

// DeleteChannel удаляет канал
func (s *channelService) DeleteChannel(ctx context.Context, id int64) error {
	s.logger.Info("Deleting channel", zap.Int64("channel_id", id))

	// Валидация входных данных
	if err := validation.ValidatePositiveInt(id, "channel ID"); err != nil {
		s.logger.Warn("Invalid channel ID provided for deletion",
			zap.Int64("channel_id", id),
			zap.Error(err),
		)
		return errors.NewValidationError("channel_id", "Channel ID must be positive").
			WithDetail("provided_value", id)
	}

	// Проверяем, существует ли канал
	existingChannel, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			s.logger.Info("Channel not found for deletion", zap.Int64("channel_id", id))
			return errors.NewChannelNotFoundError(id)
		}

		s.logger.Error("Failed to check existing channel for deletion",
			zap.Int64("channel_id", id),
			zap.Error(err),
		)
		return errors.NewDatabaseError("get channel by id for deletion", err).
			WithDetail("channel_id", id)
	}

	// Проверяем, можно ли удалить канал
	if existingChannel.Status == models.ChannelStatusActive {
		s.logger.Warn("Attempted to delete active channel",
			zap.Int64("channel_id", id),
			zap.String("status", existingChannel.Status),
		)
		return errors.NewForbiddenError("Cannot delete active channel").
			WithDetail("channel_id", id).
			WithDetail("current_status", existingChannel.Status)
	}

	// Удаляем канал
	if err := s.repo.Delete(ctx, id); err != nil {
		s.logger.Error("Failed to delete channel",
			zap.Int64("channel_id", id),
			zap.Error(err),
		)
		return errors.NewDatabaseError("delete channel", err).
			WithDetail("channel_id", id)
	}

	s.logger.Info("Channel deleted successfully",
		zap.Int64("channel_id", id),
		zap.String("title", existingChannel.Title),
	)

	return nil
}

// BanChannel блокирует канал
func (s *channelService) BanChannel(ctx context.Context, id int64, reason string) error {
	s.logger.Info("Banning channel",
		zap.Int64("channel_id", id),
		zap.String("reason", reason),
	)

	// Валидация входных данных
	if err := validation.ValidatePositiveInt(id, "channel ID"); err != nil {
		s.logger.Warn("Invalid channel ID provided for ban",
			zap.Int64("channel_id", id),
			zap.Error(err),
		)
		return errors.NewValidationError("channel_id", "Channel ID must be positive").
			WithDetail("provided_value", id)
	}

	// Проверяем, существует ли канал
	existingChannel, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			s.logger.Info("Channel not found for ban", zap.Int64("channel_id", id))
			return errors.NewChannelNotFoundError(id)
		}

		s.logger.Error("Failed to check existing channel for ban",
			zap.Int64("channel_id", id),
			zap.Error(err),
		)
		return errors.NewDatabaseError("get channel by id for ban", err).
			WithDetail("channel_id", id)
	}

	// Проверяем текущий статус
	if existingChannel.Status == models.ChannelStatusBanned {
		s.logger.Info("Channel is already banned",
			zap.Int64("channel_id", id),
			zap.String("current_status", existingChannel.Status),
		)
		return errors.NewConflictError("channel status", "Channel is already banned").
			WithDetail("channel_id", id).
			WithDetail("current_status", existingChannel.Status)
	}

	// Обновляем статус на заблокированный
	existingChannel.Status = models.ChannelStatusBanned
	existingChannel.UpdatedAt = time.Now()

	if err := s.repo.Update(ctx, existingChannel); err != nil {
		s.logger.Error("Failed to ban channel",
			zap.Int64("channel_id", id),
			zap.Error(err),
		)
		return errors.NewDatabaseError("update channel status to banned", err).
			WithDetail("channel_id", id)
	}

	s.logger.Info("Channel banned successfully",
		zap.Int64("channel_id", id),
		zap.String("title", existingChannel.Title),
		zap.String("reason", reason),
	)

	return nil
}

// UnbanChannel разблокирует канал
func (s *channelService) UnbanChannel(ctx context.Context, id int64) error {
	s.logger.Info("Unbanning channel", zap.Int64("channel_id", id))

	// Валидация входных данных
	if err := validation.ValidatePositiveInt(id, "channel ID"); err != nil {
		s.logger.Warn("Invalid channel ID provided for unban",
			zap.Int64("channel_id", id),
			zap.Error(err),
		)
		return errors.NewValidationError("channel_id", "Channel ID must be positive").
			WithDetail("provided_value", id)
	}

	// Проверяем, существует ли канал
	existingChannel, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			s.logger.Info("Channel not found for unban", zap.Int64("channel_id", id))
			return errors.NewChannelNotFoundError(id)
		}

		s.logger.Error("Failed to check existing channel for unban",
			zap.Int64("channel_id", id),
			zap.Error(err),
		)
		return errors.NewDatabaseError("get channel by id for unban", err).
			WithDetail("channel_id", id)
	}

	// Проверяем текущий статус
	if existingChannel.Status != models.ChannelStatusBanned {
		s.logger.Info("Channel is not banned",
			zap.Int64("channel_id", id),
			zap.String("current_status", existingChannel.Status),
		)
		return errors.NewConflictError("channel status", "Channel is not banned").
			WithDetail("channel_id", id).
			WithDetail("current_status", existingChannel.Status)
	}

	// Обновляем статус на активный
	existingChannel.Status = models.ChannelStatusActive
	existingChannel.UpdatedAt = time.Now()

	if err := s.repo.Update(ctx, existingChannel); err != nil {
		s.logger.Error("Failed to unban channel",
			zap.Int64("channel_id", id),
			zap.Error(err),
		)
		return errors.NewDatabaseError("update channel status to active", err).
			WithDetail("channel_id", id)
	}

	s.logger.Info("Channel unbanned successfully",
		zap.Int64("channel_id", id),
		zap.String("title", existingChannel.Title),
	)

	return nil
}

// ListChannels получает список каналов с пагинацией
func (s *channelService) ListChannels(ctx context.Context, offset, limit int) ([]*models.Channel, error) {
	s.logger.Debug("Listing channels",
		zap.Int("offset", offset),
		zap.Int("limit", limit),
	)

	// Валидация параметров пагинации
	if offset < 0 {
		return nil, errors.NewValidationError("offset", "Offset must be non-negative").
			WithDetail("provided_offset", offset)
	}

	if limit <= 0 || limit > 100 {
		return nil, errors.NewValidationError("limit", "Limit must be between 1 and 100").
			WithDetail("provided_limit", limit)
	}

	channels, err := s.repo.List(ctx, offset, limit)
	if err != nil {
		s.logger.Error("Failed to list channels",
			zap.Int("offset", offset),
			zap.Int("limit", limit),
			zap.Error(err),
		)
		return nil, errors.NewDatabaseError("list channels", err).
			WithDetail("offset", offset).
			WithDetail("limit", limit)
	}

	s.logger.Debug("Channels listed successfully",
		zap.Int("count", len(channels)),
		zap.Int("offset", offset),
		zap.Int("limit", limit),
	)

	return channels, nil
}

// GetChannelStats получает статистику канала
func (s *channelService) GetChannelStats(ctx context.Context, channelID int64) (*models.ChannelStats, error) {
	s.logger.Debug("Getting channel stats", zap.Int64("channel_id", channelID))

	// Валидация входных данных
	if err := validation.ValidatePositiveInt(channelID, "channel ID"); err != nil {
		s.logger.Warn("Invalid channel ID provided for stats",
			zap.Int64("channel_id", channelID),
			zap.Error(err),
		)
		return nil, errors.NewValidationError("channel_id", "Channel ID must be positive").
			WithDetail("provided_value", channelID)
	}

	// Проверяем, существует ли канал
	_, err := s.repo.GetByID(ctx, channelID)
	if err != nil {
		if err == sql.ErrNoRows {
			s.logger.Info("Channel not found for stats", zap.Int64("channel_id", channelID))
			return nil, errors.NewChannelNotFoundError(channelID)
		}

		s.logger.Error("Failed to check channel existence for stats",
			zap.Int64("channel_id", channelID),
			zap.Error(err),
		)
		return nil, errors.NewDatabaseError("get channel by id for stats", err).
			WithDetail("channel_id", channelID)
	}

	stats, err := s.repo.GetChannelStats(ctx, channelID)
	if err != nil {
		s.logger.Error("Failed to get channel stats",
			zap.Int64("channel_id", channelID),
			zap.Error(err),
		)
		return nil, errors.NewDatabaseError("get channel stats", err).
			WithDetail("channel_id", channelID)
	}

	s.logger.Debug("Channel stats retrieved successfully",
		zap.Int64("channel_id", channelID),
	)

	return stats, nil
}

// validateChannelData валидирует данные канала
func (s *channelService) validateChannelData(channel *models.Channel) error {
	var validationErrors []errors.AppError

	// Валидация ID канала
	if channel.ID <= 0 {
		validationErrors = append(validationErrors, *errors.NewValidationError("channel_id", "Channel ID must be positive").
			WithDetail("provided_value", channel.ID))
	}

	// Валидация title
	if channel.Title != "" {
		if !validation.IsValidChannelTitle(channel.Title) {
			validationErrors = append(validationErrors, *errors.NewValidationError("title", "Invalid channel title format").
				WithDetail("provided_value", channel.Title))
		}
	}

	// Валидация username
	if channel.Username != "" {
		if !validation.IsValidChannelUsername(channel.Username) {
			validationErrors = append(validationErrors, *errors.NewValidationError("username", "Invalid channel username format").
				WithDetail("provided_value", channel.Username))
		}
	}

	// Валидация status
	if channel.Status != "" {
		if !validation.IsValidChannelStatus(channel.Status) {
			validationErrors = append(validationErrors, *errors.NewValidationError("status", "Invalid channel status").
				WithDetail("provided_value", channel.Status).
				WithDetail("valid_values", []string{models.ChannelStatusActive, models.ChannelStatusInactive, models.ChannelStatusBanned}))
		}
	}

	if len(validationErrors) > 0 {
		// Возвращаем первую ошибку валидации для совместимости
		return &validationErrors[0]
	}

	return nil
}
