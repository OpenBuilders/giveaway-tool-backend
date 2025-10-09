package user

import "context"

// Repository defines persistence operations for User aggregate.
type Repository interface {
	Upsert(ctx context.Context, u *User) error
	GetByID(ctx context.Context, id int64) (*User, error)
	GetByUsername(ctx context.Context, username string) (*User, error)
	List(ctx context.Context, limit, offset int) ([]User, error)
	Delete(ctx context.Context, id int64) error
	Touch(ctx context.Context, id int64) error
}
