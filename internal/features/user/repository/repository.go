package repository

import (
	"context"
	"errors"
	"giveaway-tool-backend/internal/features/user/models"
)

var (
	ErrUserNotFound = errors.New("user not found")
)

type UserRepository interface {
	Create(ctx context.Context, user *models.User) error
	GetByID(ctx context.Context, id int64) (*models.User, error)
	Update(ctx context.Context, user *models.User) error
	Delete(ctx context.Context, id int64) error
	GetByUsername(ctx context.Context, username string) (*models.User, error)
	GetUsersByIDs(ctx context.Context, ids []int64) ([]*models.User, error)
	List(ctx context.Context, limit, offset int) ([]*models.User, error)
	UpdateStatus(ctx context.Context, id int64, status string) error
	GetUserStats(ctx context.Context, userID int64) (*models.UserStats, error)
	GetUserGiveaways(ctx context.Context, userID int64, status string) ([]*models.Giveaway, error)
	GetUserWins(ctx context.Context, userID int64) ([]*models.WinRecord, error)
}
