package giveaway

import "context"

// Repository defines persistence operations for Giveaway aggregate.
type Repository interface {
	Create(ctx context.Context, g *Giveaway) error
	GetByID(ctx context.Context, id string) (*Giveaway, error)
	ListByCreator(ctx context.Context, creatorID int64, limit, offset int) ([]Giveaway, error)
	UpdateStatus(ctx context.Context, id string, status GiveawayStatus) error
	DeleteByOwner(ctx context.Context, id string, ownerID int64) (bool, error)
}
