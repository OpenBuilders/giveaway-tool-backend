package service

import (
	"context"
	"database/sql"
	"time"

	"go.uber.org/zap"

	"giveaway-tool-backend/internal/common/errors"
	"giveaway-tool-backend/internal/common/validation"
	"giveaway-tool-backend/internal/features/user/models"
	"giveaway-tool-backend/internal/features/user/repository"
)

// userService предоставляет бизнес-логику для работы с пользователями
type userService struct {
	repo   repository.UserRepository
	logger *zap.Logger
}

// NewUserService создает новый экземпляр UserService
func NewUserService(repo repository.UserRepository, logger *zap.Logger) UserService {
	return &userService{
		repo:   repo,
		logger: logger,
	}
}

// GetUser получает пользователя по ID
func (s *userService) GetUser(ctx context.Context, id int64) (*models.UserResponse, error) {
	s.logger.Debug("Getting user by ID", zap.Int64("user_id", id))

	// Валидация входных данных
	if err := validation.ValidatePositiveInt(id, "user ID"); err != nil {
		s.logger.Warn("Invalid user ID provided",
			zap.Int64("user_id", id),
			zap.Error(err),
		)
		return nil, errors.NewValidationError("user_id", "User ID must be positive").
			WithDetail("provided_value", id)
	}

	user, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			s.logger.Info("User not found", zap.Int64("user_id", id))
			return nil, errors.NewUserNotFoundError(id)
		}

		s.logger.Error("Failed to get user by ID",
			zap.Int64("user_id", id),
			zap.Error(err),
		)
		return nil, errors.NewDatabaseError("get user by id", err).
			WithUserID(id)
	}

	s.logger.Debug("User retrieved successfully",
		zap.Int64("user_id", id),
		zap.String("username", user.Username),
	)

	// Преобразуем в UserResponse
	userResponse := &models.UserResponse{
		ID:        user.ID,
		Username:  user.Username,
		FirstName: user.FirstName,
		LastName:  user.LastName,
		Role:      user.Role,
		Status:    user.Status,
		CreatedAt: user.CreatedAt,
	}

	return userResponse, nil
}

// UpdateUserStatus обновляет статус пользователя
func (s *userService) UpdateUserStatus(ctx context.Context, id int64, status string) error {
	s.logger.Info("Updating user status",
		zap.Int64("user_id", id),
		zap.String("new_status", status),
	)

	// Валидация входных данных
	if err := validation.ValidatePositiveInt(id, "user ID"); err != nil {
		s.logger.Warn("Invalid user ID provided for status update",
			zap.Int64("user_id", id),
			zap.Error(err),
		)
		return errors.NewValidationError("user_id", "User ID must be positive").
			WithDetail("provided_value", id)
	}

	if err := validation.ValidateUserStatus(status); err != nil {
		s.logger.Warn("Invalid user status provided",
			zap.String("status", status),
			zap.Error(err),
		)
		return errors.NewValidationError("status", "Invalid user status").
			WithDetail("provided_value", status)
	}

	// Получаем пользователя
	user, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			s.logger.Info("User not found for status update", zap.Int64("user_id", id))
			return errors.NewUserNotFoundError(id)
		}

		s.logger.Error("Failed to get user for status update",
			zap.Int64("user_id", id),
			zap.Error(err),
		)
		return errors.NewDatabaseError("get user by id for status update", err).
			WithUserID(id)
	}

	// Проверяем, можно ли изменить статус
	if user.Status == models.UserStatusBanned && status != models.UserStatusBanned {
		s.logger.Warn("Attempted to change status of banned user",
			zap.Int64("user_id", id),
			zap.String("current_status", user.Status),
			zap.String("new_status", status),
		)
		return errors.NewForbiddenError("Cannot change status of banned user").
			WithUserID(id).
			WithDetail("current_status", user.Status).
			WithDetail("new_status", status)
	}

	// Обновляем статус
	user.Status = status
	user.UpdatedAt = time.Now()

	if err := s.repo.Update(ctx, user); err != nil {
		s.logger.Error("Failed to update user status",
			zap.Int64("user_id", id),
			zap.String("new_status", status),
			zap.Error(err),
		)
		return errors.NewDatabaseError("update user status", err).
			WithUserID(id)
	}

	s.logger.Info("User status updated successfully",
		zap.Int64("user_id", id),
		zap.String("username", user.Username),
		zap.String("new_status", status),
	)

	return nil
}

// GetOrCreateUser получает или создает пользователя
func (s *userService) GetOrCreateUser(ctx context.Context, telegramID int64, username, firstName, lastName string) (*models.UserResponse, error) {
	s.logger.Info("Getting or creating user",
		zap.Int64("telegram_id", telegramID),
		zap.String("username", username),
	)

	// Валидация входных данных
	if err := validation.ValidatePositiveInt(telegramID, "telegram ID"); err != nil {
		s.logger.Warn("Invalid telegram ID provided",
			zap.Int64("telegram_id", telegramID),
			zap.Error(err),
		)
		return nil, errors.NewValidationError("telegram_id", "Telegram ID must be positive").
			WithDetail("provided_value", telegramID)
	}

	if err := validation.ValidateUsername(username); err != nil {
		s.logger.Warn("Invalid username provided",
			zap.String("username", username),
			zap.Error(err),
		)
		return nil, errors.NewValidationError("username", "Invalid username format").
			WithDetail("provided_value", username)
	}

	if err := validation.ValidateFirstName(firstName); err != nil {
		s.logger.Warn("Invalid first name provided",
			zap.String("first_name", firstName),
			zap.Error(err),
		)
		return nil, errors.NewValidationError("first_name", "Invalid first name format").
			WithDetail("provided_value", firstName)
	}

	if err := validation.ValidateLastName(lastName); err != nil {
		s.logger.Warn("Invalid last name provided",
			zap.String("last_name", lastName),
			zap.Error(err),
		)
		return nil, errors.NewValidationError("last_name", "Invalid last name format").
			WithDetail("provided_value", lastName)
	}

	// Пытаемся получить существующего пользователя
	user, err := s.repo.GetByID(ctx, telegramID)
	if err == nil {
		// Пользователь существует, обновляем данные если изменились
		updated := false
		if user.Username != username {
			user.Username = username
			updated = true
		}
		if user.FirstName != firstName {
			user.FirstName = firstName
			updated = true
		}
		if user.LastName != lastName {
			user.LastName = lastName
			updated = true
		}

		if updated {
			user.UpdatedAt = time.Now()
			if err := s.repo.Update(ctx, user); err != nil {
				s.logger.Error("Failed to update existing user",
					zap.Int64("user_id", telegramID),
					zap.Error(err),
				)
				return nil, errors.NewDatabaseError("update existing user", err).
					WithUserID(telegramID)
			}
			s.logger.Info("Updated existing user",
				zap.Int64("user_id", telegramID),
				zap.String("username", username),
			)
		}

		// Преобразуем в UserResponse
		userResponse := &models.UserResponse{
			ID:        user.ID,
			Username:  user.Username,
			FirstName: user.FirstName,
			LastName:  user.LastName,
			Role:      user.Role,
			Status:    user.Status,
			CreatedAt: user.CreatedAt,
		}

		return userResponse, nil
	}

	// Пользователь не найден, создаем нового
	if err != sql.ErrNoRows {
		s.logger.Error("Failed to check existing user",
			zap.Int64("telegram_id", telegramID),
			zap.Error(err),
		)
		return nil, errors.NewDatabaseError("get user by id for create", err).
			WithUserID(telegramID)
	}

	// Создаем нового пользователя
	newUser := &models.User{
		ID:        telegramID,
		Username:  username,
		FirstName: firstName,
		LastName:  lastName,
		Role:      "user",
		Status:    "active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.repo.Create(ctx, newUser); err != nil {
		s.logger.Error("Failed to create new user",
			zap.Int64("telegram_id", telegramID),
			zap.String("username", username),
			zap.Error(err),
		)
		return nil, errors.NewDatabaseError("create new user", err).
			WithUserID(telegramID)
	}

	s.logger.Info("Created new user",
		zap.Int64("user_id", telegramID),
		zap.String("username", username),
	)

	// Преобразуем в UserResponse
	userResponse := &models.UserResponse{
		ID:        newUser.ID,
		Username:  newUser.Username,
		FirstName: newUser.FirstName,
		LastName:  newUser.LastName,
		Role:      newUser.Role,
		Status:    newUser.Status,
		CreatedAt: newUser.CreatedAt,
	}

	return userResponse, nil
}

// GetUserStats получает статистику пользователя
func (s *userService) GetUserStats(ctx context.Context, userID int64) (*models.UserStats, error) {
	s.logger.Debug("Getting user stats", zap.Int64("user_id", userID))

	// Валидация входных данных
	if err := validation.ValidatePositiveInt(userID, "user ID"); err != nil {
		s.logger.Warn("Invalid user ID provided for stats",
			zap.Int64("user_id", userID),
			zap.Error(err),
		)
		return nil, errors.NewValidationError("user_id", "User ID must be positive").
			WithDetail("provided_value", userID)
	}

	// Проверяем, существует ли пользователь
	_, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			s.logger.Info("User not found for stats", zap.Int64("user_id", userID))
			return nil, errors.NewUserNotFoundError(userID)
		}

		s.logger.Error("Failed to check user existence for stats",
			zap.Int64("user_id", userID),
			zap.Error(err),
		)
		return nil, errors.NewDatabaseError("get user by id for stats", err).
			WithUserID(userID)
	}

	stats, err := s.repo.GetUserStats(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to get user stats",
			zap.Int64("user_id", userID),
			zap.Error(err),
		)
		return nil, errors.NewDatabaseError("get user stats", err).
			WithUserID(userID)
	}

	s.logger.Debug("User stats retrieved successfully",
		zap.Int64("user_id", userID),
	)

	return stats, nil
}

// GetUserGiveaways получает гивы пользователя
func (s *userService) GetUserGiveaways(ctx context.Context, userID int64, status string) ([]*models.Giveaway, error) {
	s.logger.Debug("Getting user giveaways",
		zap.Int64("user_id", userID),
		zap.String("status", status),
	)

	// Валидация входных данных
	if err := validation.ValidatePositiveInt(userID, "user ID"); err != nil {
		s.logger.Warn("Invalid user ID provided for giveaways",
			zap.Int64("user_id", userID),
			zap.Error(err),
		)
		return nil, errors.NewValidationError("user_id", "User ID must be positive").
			WithDetail("provided_value", userID)
	}

	if status != "" && len(status) > 50 {
		s.logger.Warn("Status too long for giveaways",
			zap.String("status", status),
			zap.Int("length", len(status)),
		)
		return nil, errors.NewValidationError("status", "Status too long").
			WithDetail("provided_value", status).
			WithDetail("max_length", 50)
	}

	// Проверяем, существует ли пользователь
	_, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			s.logger.Info("User not found for giveaways", zap.Int64("user_id", userID))
			return nil, errors.NewUserNotFoundError(userID)
		}

		s.logger.Error("Failed to check user existence for giveaways",
			zap.Int64("user_id", userID),
			zap.Error(err),
		)
		return nil, errors.NewDatabaseError("get user by id for giveaways", err).
			WithUserID(userID)
	}

	giveaways, err := s.repo.GetUserGiveaways(ctx, userID, status)
	if err != nil {
		s.logger.Error("Failed to get user giveaways",
			zap.Int64("user_id", userID),
			zap.String("status", status),
			zap.Error(err),
		)
		return nil, errors.NewDatabaseError("get user giveaways", err).
			WithUserID(userID)
	}

	s.logger.Debug("User giveaways retrieved successfully",
		zap.Int64("user_id", userID),
		zap.Int("count", len(giveaways)),
	)

	return giveaways, nil
}

// GetUserWins получает победы пользователя
func (s *userService) GetUserWins(ctx context.Context, userID int64) ([]*models.WinRecord, error) {
	s.logger.Debug("Getting user wins", zap.Int64("user_id", userID))

	// Валидация входных данных
	if err := validation.ValidatePositiveInt(userID, "user ID"); err != nil {
		s.logger.Warn("Invalid user ID provided for wins",
			zap.Int64("user_id", userID),
			zap.Error(err),
		)
		return nil, errors.NewValidationError("user_id", "User ID must be positive").
			WithDetail("provided_value", userID)
	}

	// Проверяем, существует ли пользователь
	_, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			s.logger.Info("User not found for wins", zap.Int64("user_id", userID))
			return nil, errors.NewUserNotFoundError(userID)
		}

		s.logger.Error("Failed to check user existence for wins",
			zap.Int64("user_id", userID),
			zap.Error(err),
		)
		return nil, errors.NewDatabaseError("get user by id for wins", err).
			WithUserID(userID)
	}

	wins, err := s.repo.GetUserWins(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to get user wins",
			zap.Int64("user_id", userID),
			zap.Error(err),
		)
		return nil, errors.NewDatabaseError("get user wins", err).
			WithUserID(userID)
	}

	s.logger.Debug("User wins retrieved successfully",
		zap.Int64("user_id", userID),
		zap.Int("count", len(wins)),
	)

	return wins, nil
}
