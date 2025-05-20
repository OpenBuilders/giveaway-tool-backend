package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"giveaway-tool-backend/internal/features/tonproof/models"
	"time"

	"github.com/redis/go-redis/v9"
)

type redisRepository struct {
	client *redis.Client
}

func NewRedisRepository(client *redis.Client) Repository {
	return &redisRepository{
		client: client,
	}
}

func (r *redisRepository) SaveState(ctx context.Context, userID int64, state *models.TONProofState) error {
	key := fmt.Sprintf("tonproof:state:%d", userID)

	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// Сохраняем состояние на 5 минут
	if err := r.client.Set(ctx, key, data, 5*time.Minute).Err(); err != nil {
		return fmt.Errorf("failed to save state to redis: %w", err)
	}

	return nil
}

func (r *redisRepository) GetState(ctx context.Context, userID int64) (*models.TONProofState, error) {
	key := fmt.Sprintf("tonproof:state:%d", userID)

	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("state not found")
		}
		return nil, fmt.Errorf("failed to get state from redis: %w", err)
	}

	var state models.TONProofState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state: %w", err)
	}

	return &state, nil
}

func (r *redisRepository) SaveProof(ctx context.Context, record *models.TONProofRecord) error {
	key := fmt.Sprintf("tonproof:proof:%d", record.UserID)

	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal proof: %w", err)
	}

	if err := r.client.Set(ctx, key, data, 24*time.Hour).Err(); err != nil {
		return fmt.Errorf("failed to save proof to redis: %w", err)
	}

	return nil
}

func (r *redisRepository) GetProof(ctx context.Context, userID int64) (*models.TONProofRecord, error) {
	key := fmt.Sprintf("tonproof:proof:%d", userID)

	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("proof not found")
		}
		return nil, fmt.Errorf("failed to get proof from redis: %w", err)
	}

	var record models.TONProofRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("failed to unmarshal proof: %w", err)
	}

	return &record, nil
}

func (r *redisRepository) IsProofValid(ctx context.Context, userID int64) (bool, error) {
	record, err := r.GetProof(ctx, userID)
	if err != nil {
		return false, err
	}

	return record != nil, nil
}

func (r *redisRepository) DeleteProof(ctx context.Context, userID int64) error {
	key := fmt.Sprintf("tonproof:proof:%d", userID)

	if err := r.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to delete proof from redis: %w", err)
	}

	return nil
}

func (r *redisRepository) GenerateState(ctx context.Context, userID int64) (*models.TONProofState, error) {
	state := &models.TONProofState{
		UserID:    userID,
		State:     fmt.Sprintf("%d", time.Now().UnixNano()),
		CreatedAt: time.Now(),
	}

	if err := r.SaveState(ctx, userID, state); err != nil {
		return nil, fmt.Errorf("failed to save state: %w", err)
	}

	return state, nil
}
