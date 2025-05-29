package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"giveaway-tool-backend/internal/features/tonproof/models"
	"giveaway-tool-backend/internal/features/tonproof/repository"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	keyPrefixProof  = "ton_proof:"
	keyPrefixState  = "ton_state:"
	proofExpiration = 30 * 24 * time.Hour // 30 дней
)

type Repository struct {
	client *redis.Client
}

func NewRepository(client *redis.Client) repository.Repository {
	return &Repository{client: client}
}

func (r *Repository) SaveProof(ctx context.Context, record *models.TONProofRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal proof record: %w", err)
	}

	key := fmt.Sprintf("%s%d", keyPrefixProof, record.UserID)
	return r.client.Set(ctx, key, data, proofExpiration).Err()
}

func (r *Repository) GetProof(ctx context.Context, userID int64) (*models.TONProofRecord, error) {
	key := fmt.Sprintf("%s%d", keyPrefixProof, userID)
	data, err := r.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get proof: %w", err)
	}

	var record models.TONProofRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("failed to unmarshal proof record: %w", err)
	}

	return &record, nil
}

func (r *Repository) GenerateState(ctx context.Context, userID int64) (*models.TONProofState, error) {
	state := &models.TONProofState{
		UserID:    userID,
		State:     uuid.New().String(),
		CreatedAt: time.Now(),
	}

	data, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state: %w", err)
	}

	key := fmt.Sprintf("%s%d", keyPrefixState, userID)
	if err := r.client.Set(ctx, key, data, 15*time.Minute).Err(); err != nil {
		return nil, fmt.Errorf("failed to save state: %w", err)
	}

	return state, nil
}

func (r *Repository) GetState(ctx context.Context, userID int64) (*models.TONProofState, error) {
	key := fmt.Sprintf("%s%d", keyPrefixState, userID)
	data, err := r.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get state: %w", err)
	}

	var state models.TONProofState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state: %w", err)
	}

	return &state, nil
}
