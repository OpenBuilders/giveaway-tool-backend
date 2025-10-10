package user

import (
	"context"
	"errors"
	"time"

	rcache "github.com/open-builders/giveaway-backend/internal/cache/redis"
	domain "github.com/open-builders/giveaway-backend/internal/domain/user"
	pgrepo "github.com/open-builders/giveaway-backend/internal/repository/postgres"
)

// Service orchestrates user access with repository and cache.
type Service struct {
	repo  *pgrepo.UserRepository
	cache *rcache.UserCache
}

func NewService(repo *pgrepo.UserRepository, cache *rcache.UserCache) *Service {
	return &Service{repo: repo, cache: cache}
}

func (s *Service) GetByID(ctx context.Context, id int64) (*domain.User, error) {
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

func (s *Service) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
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

func (s *Service) Upsert(ctx context.Context, u *domain.User) error {
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

func (s *Service) Delete(ctx context.Context, id int64) error {
	u, _ := s.repo.GetByID(ctx, id)
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	if s.cache != nil && u != nil {
		_ = s.cache.Invalidate(ctx, u)
	}
	return nil
}

func (s *Service) List(ctx context.Context, limit, offset int) ([]domain.User, error) {
	return s.repo.List(ctx, limit, offset)
}
