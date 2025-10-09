package service

import (
	"context"
	"errors"
	"time"

	domain "github.com/your-org/giveaway-backend/internal/domain/user"
	pgrepo "github.com/your-org/giveaway-backend/internal/repository/postgres"
	rcache "github.com/your-org/giveaway-backend/internal/cache/redis"
)

// UserService orchestrates user access with repository and cache.
type UserService struct {
	repo  *pgrepo.UserRepository
	cache *rcache.UserCache
	cacheTTL time.Duration
}

func NewUserService(repo *pgrepo.UserRepository, cache *rcache.UserCache, ttl time.Duration) *UserService {
	return &UserService{repo: repo, cache: cache, cacheTTL: ttl}
}

func (s *UserService) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	if s.cache != nil {
		if u, err := s.cache.GetByID(ctx, id); err == nil && u != nil {
			return u, nil
		}
	}
	u, err := s.repo.GetByID(ctx, id)
	if err != nil || u == nil {
		return u, err
	}
	if s.cache != nil {
		_ = s.cache.Set(ctx, u)
	}
	return u, nil
}

func (s *UserService) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	if s.cache != nil {
		if u, err := s.cache.GetByUsername(ctx, username); err == nil && u != nil {
			return u, nil
		}
	}
	u, err := s.repo.GetByUsername(ctx, username)
	if err != nil || u == nil {
		return u, err
	}
	if s.cache != nil {
		_ = s.cache.Set(ctx, u)
	}
	return u, nil
}

func (s *UserService) Upsert(ctx context.Context, u *domain.User) error {
	if u == nil {
		return errors.New("nil user")
	}
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now().UTC()
	}
	u.UpdatedAt = time.Now().UTC()
	if err := s.repo.Upsert(ctx, u); err != nil {
		return err
	}
	if s.cache != nil {
		_ = s.cache.Set(ctx, u)
	}
	return nil
}

func (s *UserService) Delete(ctx context.Context, id int64) error {
	u, _ := s.repo.GetByID(ctx, id)
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	if s.cache != nil && u != nil {
		_ = s.cache.Invalidate(ctx, u)
	}
	return nil
}

func (s *UserService) List(ctx context.Context, limit, offset int) ([]domain.User, error) {
	return s.repo.List(ctx, limit, offset)
}
