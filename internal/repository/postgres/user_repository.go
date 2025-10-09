package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	domain "github.com/your-org/giveaway-backend/internal/domain/user"
)

// UserRepository provides CRUD operations for users in Postgres.
type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository { return &UserRepository{db: db} }

// Upsert inserts or updates a user by ID. Username uniqueness is case-insensitive when present.
func (r *UserRepository) Upsert(ctx context.Context, u *domain.User) error {
	const q = `
	INSERT INTO users (id, username, first_name, last_name, role, status, created_at, updated_at)
	VALUES ($1, lower(NULLIF($2, '')), $3, $4, $5, $6, COALESCE($7, now()), COALESCE($8, now()))
	ON CONFLICT (id) DO UPDATE SET
		username = EXCLUDED.username,
		first_name = EXCLUDED.first_name,
		last_name = EXCLUDED.last_name,
		role = EXCLUDED.role,
		status = EXCLUDED.status,
		updated_at = now();
	`
	_, err := r.db.ExecContext(ctx, q,
		u.ID,
		u.Username,
		u.FirstName,
		u.LastName,
		u.Role,
		u.Status,
		u.CreatedAt,
		u.UpdatedAt,
	)
	return err
}

// GetByID returns a user by Telegram ID.
func (r *UserRepository) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	const q = `SELECT id, username, first_name, last_name, role, status, created_at, updated_at FROM users WHERE id=$1`
	row := r.db.QueryRowContext(ctx, q, id)
	var u domain.User
	if err := row.Scan(&u.ID, &u.Username, &u.FirstName, &u.LastName, &u.Role, &u.Status, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

// GetByUsername returns a user by username (case-insensitive). Returns nil if not found.
func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	const q = `
	SELECT id, username, first_name, last_name, role, status, created_at, updated_at
	FROM users
	WHERE lower(username) = lower($1)
	`
	row := r.db.QueryRowContext(ctx, q, username)
	var u domain.User
	if err := row.Scan(&u.ID, &u.Username, &u.FirstName, &u.LastName, &u.Role, &u.Status, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

// List returns users with pagination ordered by created_at desc.
func (r *UserRepository) List(ctx context.Context, limit, offset int) ([]domain.User, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	const q = `
	SELECT id, username, first_name, last_name, role, status, created_at, updated_at
	FROM users
	ORDER BY created_at DESC
	LIMIT $1 OFFSET $2`
	rows, err := r.db.QueryContext(ctx, q, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []domain.User
	for rows.Next() {
		var u domain.User
		if err := rows.Scan(&u.ID, &u.Username, &u.FirstName, &u.LastName, &u.Role, &u.Status, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// Delete removes a user by ID.
func (r *UserRepository) Delete(ctx context.Context, id int64) error {
	const q = `DELETE FROM users WHERE id=$1`
	_, err := r.db.ExecContext(ctx, q, id)
	return err
}

// Touch updates updated_at without changing other fields; useful for heartbeat-style updates.
func (r *UserRepository) Touch(ctx context.Context, id int64) error {
	const q = `UPDATE users SET updated_at=now() WHERE id=$1`
	_, err := r.db.ExecContext(ctx, q, id)
	return err
}

// Ensure compiles with a usage to time to avoid removal by formatters
var _ = time.Now
