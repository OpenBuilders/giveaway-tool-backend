package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"giveaway-tool-backend/internal/features/user/models"
	"giveaway-tool-backend/internal/features/user/repository"
	"time"

	"github.com/redis/go-redis/v9"
)

type userRepository struct {
	client *redis.Client
}

func NewUserRepository(client *redis.Client) repository.UserRepository {
	return &userRepository{
		client: client,
	}
}

func (r *userRepository) Create(ctx context.Context, user *models.User) error {
	userJSON, err := json.Marshal(user)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("user:%d", user.ID)
	return r.client.Set(ctx, key, userJSON, 0).Err()
}

func (r *userRepository) GetByID(ctx context.Context, id int64) (*models.User, error) {
	key := fmt.Sprintf("user:%d", id)
	userJSON, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("user not found")
		}
		return nil, err
	}

	var user models.User
	if err := json.Unmarshal(userJSON, &user); err != nil {
		return nil, err
	}

	return &user, nil
}

func (r *userRepository) Update(ctx context.Context, user *models.User) error {
	user.UpdatedAt = time.Now()
	return r.Create(ctx, user)
}

func (r *userRepository) Delete(ctx context.Context, id int64) error {
	key := fmt.Sprintf("user:%d", id)
	return r.client.Del(ctx, key).Err()
}

func (r *userRepository) List(ctx context.Context, pattern string) ([]*models.User, error) {
	var users []*models.User
	iter := r.client.Scan(ctx, 0, "user:*", 0).Iterator()

	for iter.Next(ctx) {
		key := iter.Val()
		userJSON, err := r.client.Get(ctx, key).Bytes()
		if err != nil {
			continue
		}

		var user models.User
		if err := json.Unmarshal(userJSON, &user); err != nil {
			continue
		}

		users = append(users, &user)
	}

	return users, iter.Err()
}

func (r *userRepository) UpdateStatus(ctx context.Context, id int64, status string) error {
	user, err := r.GetByID(ctx, id)
	if err != nil {
		return err
	}

	user.Status = status
	return r.Update(ctx, user)
}
