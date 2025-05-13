package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"

	"giveaway-tool-backend/internal/model"
)

// UserRepo хранит User-ы только в Redis (ключ "user:<id>").
type UserRepo struct {
	rdb *redis.Client
}

func NewUserRepo(rdb *redis.Client) *UserRepo {
	return &UserRepo{rdb: rdb}
}

func (r *UserRepo) Save(ctx context.Context, u *model.User) error {
	raw, _ := json.Marshal(u)
	return r.rdb.Set(ctx, key(u.ID), raw, 0).Err()
}

func (r *UserRepo) Get(ctx context.Context, id string) (*model.User, error) {
	raw, err := r.rdb.Get(ctx, key(id)).Result()
	if err != nil {
		return nil, err
	}
	var u model.User
	if err := json.Unmarshal([]byte(raw), &u); err != nil {
		return nil, err
	}
	return &u, nil
}

func key(id string) string { return fmt.Sprintf("user:%s", id) }
