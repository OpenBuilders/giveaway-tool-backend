package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"giveaway-tool-backend/internal/features/tonproof/models"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	keyPrefixTONProof = "tonproof:"
	proofExpiration   = 24 * time.Hour
)

type Repository struct {
	client *redis.Client
}

func NewRepository(client *redis.Client) *Repository {
	return &Repository{client: client}
}

func makeTONProofKey(userID int64) string {
	return fmt.Sprintf("%s%d", keyPrefixTONProof, userID)
}

func (r *Repository) SaveProof(ctx context.Context, userID int64, proof *models.TONProofRecord) error {
	data, err := json.Marshal(proof)
	if err != nil {
		return fmt.Errorf("failed to marshal proof: %w", err)
	}

	return r.client.Set(ctx, makeTONProofKey(userID), data, proofExpiration).Err()
}

func (r *Repository) GetProof(ctx context.Context, userID int64) (*models.TONProofRecord, error) {
	data, err := r.client.Get(ctx, makeTONProofKey(userID)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get proof: %w", err)
	}

	var proof models.TONProofRecord
	if err := json.Unmarshal(data, &proof); err != nil {
		return nil, fmt.Errorf("failed to unmarshal proof: %w", err)
	}

	return &proof, nil
}

func (r *Repository) DeleteProof(ctx context.Context, userID int64) error {
	return r.client.Del(ctx, makeTONProofKey(userID)).Err()
}

func (r *Repository) IsProofValid(ctx context.Context, userID int64) (bool, error) {
	proof, err := r.GetProof(ctx, userID)
	if err != nil {
		return false, err
	}
	if proof == nil {
		return false, nil
	}

	return proof.IsValid && time.Now().Before(proof.ExpiresAt), nil
}
