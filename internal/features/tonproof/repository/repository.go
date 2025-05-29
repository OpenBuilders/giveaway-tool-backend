package repository

import (
	"context"
	"giveaway-tool-backend/internal/features/tonproof/models"
)

type Repository interface {
	// SaveProof сохраняет запись о верификации
	SaveProof(ctx context.Context, record *models.TONProofRecord) error

	// GetProof получает запись о верификации по ID пользователя
	GetProof(ctx context.Context, userID int64) (*models.TONProofRecord, error)

	// GenerateState генерирует новое состояние для верификации
	GenerateState(ctx context.Context, userID int64) (*models.TONProofState, error)

	// GetState получает состояние по ID пользователя
	GetState(ctx context.Context, userID int64) (*models.TONProofState, error)
}
