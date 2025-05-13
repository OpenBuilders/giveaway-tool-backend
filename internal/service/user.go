package service

import (
	"context"
	"time"

	"giveaway-tool-backend/internal/model"
	"giveaway-tool-backend/internal/repository"
)

type UserService struct {
	repo *repository.UserRepo
}

func NewUserService(repo *repository.UserRepo) *UserService {
	return &UserService{repo: repo}
}

func (s *UserService) Create(ctx context.Context, id, username, firstName, lastName string, isPremium bool, avatar string) (*model.User, error) {
	u := &model.User{
		ID:        id,
		Username:  username,
		FirstName: firstName,
		LastName:  lastName,
		IsPremium: isPremium,
		Avatar:    avatar,
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	return u, s.repo.Save(ctx, u)
}

func (s *UserService) Get(ctx context.Context, id string) (*model.User, error) {
	return s.repo.Get(ctx, id)
}
