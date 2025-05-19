package service

import (
	"context"
	"errors"
	"giveaway-tool-backend/internal/features/user/models"
	"giveaway-tool-backend/internal/features/user/repository"
	"time"
)

var (
	ErrUserNotFound = errors.New("user not found")
)

type UserService interface {
	GetUser(ctx context.Context, id int64) (*models.UserResponse, error)
	UpdateUserStatus(ctx context.Context, id int64, status string) error
	GetOrCreateUser(ctx context.Context, telegramID int64, username, firstName, lastName string) (*models.UserResponse, error)
}

type userService struct {
	repo repository.UserRepository
}

func NewUserService(repo repository.UserRepository) UserService {
	return &userService{
		repo: repo,
	}
}

func (s *userService) GetUser(ctx context.Context, id int64) (*models.UserResponse, error) {
	user, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, ErrUserNotFound
	}

	return toUserResponse(user), nil
}

func (s *userService) UpdateUserStatus(ctx context.Context, id int64, status string) error {
	user, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return ErrUserNotFound
	}

	user.Status = status
	user.UpdatedAt = time.Now()

	return s.repo.Update(ctx, user)
}

func (s *userService) GetOrCreateUser(ctx context.Context, telegramID int64, username, firstName, lastName string) (*models.UserResponse, error) {
	user, err := s.repo.GetByID(ctx, telegramID)
	if err == nil {
		if user.Username != username || user.FirstName != firstName || user.LastName != lastName {
			user.Username = username
			user.FirstName = firstName
			user.LastName = lastName
			user.UpdatedAt = time.Now()
			if err := s.repo.Update(ctx, user); err != nil {
				return nil, err
			}
		}
		return toUserResponse(user), nil
	}

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
		return nil, err
	}

	return toUserResponse(newUser), nil
}

func toUserResponse(user *models.User) *models.UserResponse {
	return &models.UserResponse{
		ID:        user.ID,
		Username:  user.Username,
		FirstName: user.FirstName,
		LastName:  user.LastName,
		Role:      user.Role,
		Status:    user.Status,
		CreatedAt: user.CreatedAt,
	}
}
