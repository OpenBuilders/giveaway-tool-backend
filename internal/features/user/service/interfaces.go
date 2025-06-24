package service

import (
	"context"
	"giveaway-tool-backend/internal/features/user/models"
)

type UserService interface {
	GetUser(ctx context.Context, id int64) (*models.UserResponse, error)
	UpdateUserStatus(ctx context.Context, id int64, status string) error
	GetOrCreateUser(ctx context.Context, telegramID int64, username, firstName, lastName string) (*models.UserResponse, error)
	GetUserStats(ctx context.Context, userID int64) (*models.UserStats, error)
	GetUserGiveaways(ctx context.Context, userID int64, status string) ([]*models.Giveaway, error)
	GetUserWins(ctx context.Context, userID int64) ([]*models.WinRecord, error)
}
