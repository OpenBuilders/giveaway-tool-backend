package repository

import (
	"context"
	"giveaway-tool-backend/internal/features/user/models"
)

type UserRepository interface {
	Create(ctx context.Context, user *models.User) error
	GetByID(ctx context.Context, id int64) (*models.User, error)
	Update(ctx context.Context, user *models.User) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, pattern string) ([]*models.User, error)
	UpdateStatus(ctx context.Context, id int64, status string) error
}
