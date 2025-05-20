package repository

import (
	"context"
	"giveaway-tool-backend/internal/features/tonproof/models"
)

type Repository interface {
	SaveProof(ctx context.Context, userID int64, proof *models.TONProofRecord) error
	GetProof(ctx context.Context, userID int64) (*models.TONProofRecord, error)
	DeleteProof(ctx context.Context, userID int64) error
	IsProofValid(ctx context.Context, userID int64) (bool, error)
}
