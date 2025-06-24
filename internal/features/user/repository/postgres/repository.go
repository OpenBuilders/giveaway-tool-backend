package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"giveaway-tool-backend/internal/features/user/models"
	"giveaway-tool-backend/internal/features/user/repository"

	_ "github.com/lib/pq"
)

var (
	ErrUserNotFound = errors.New("user not found")
)

type postgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) repository.UserRepository {
	return &postgresRepository{db: db}
}

// Create создает нового пользователя
func (r *postgresRepository) Create(ctx context.Context, user *models.User) error {
	query := `
		INSERT INTO users (id, username, first_name, last_name)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE SET
			username = EXCLUDED.username,
			first_name = EXCLUDED.first_name,
			last_name = EXCLUDED.last_name,
			updated_at = NOW()
	`

	_, err := r.db.ExecContext(ctx, query,
		user.ID, user.Username, user.FirstName, user.LastName)
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

// GetByID получает пользователя по ID
func (r *postgresRepository) GetByID(ctx context.Context, id int64) (*models.User, error) {
	query := `
		SELECT id, username, first_name, last_name, created_at, updated_at
		FROM users
		WHERE id = $1
	`

	var user models.User
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID, &user.Username, &user.FirstName, &user.LastName,
		&user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, repository.ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &user, nil
}

// Update обновляет пользователя
func (r *postgresRepository) Update(ctx context.Context, user *models.User) error {
	query := `
		UPDATE users 
		SET username = $2, first_name = $3, last_name = $4, updated_at = NOW()
		WHERE id = $1
	`

	result, err := r.db.ExecContext(ctx, query,
		user.ID, user.Username, user.FirstName, user.LastName)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repository.ErrUserNotFound
	}

	return nil
}

// Delete удаляет пользователя
func (r *postgresRepository) Delete(ctx context.Context, id int64) error {
	query := "DELETE FROM users WHERE id = $1"

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repository.ErrUserNotFound
	}

	return nil
}

// GetByUsername получает пользователя по username
func (r *postgresRepository) GetByUsername(ctx context.Context, username string) (*models.User, error) {
	query := `
		SELECT id, username, first_name, last_name, created_at, updated_at
		FROM users
		WHERE username = $1
	`

	var user models.User
	err := r.db.QueryRowContext(ctx, query, username).Scan(
		&user.ID, &user.Username, &user.FirstName, &user.LastName,
		&user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, repository.ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by username: %w", err)
	}

	return &user, nil
}

// GetUsersByIDs получает пользователей по списку ID
func (r *postgresRepository) GetUsersByIDs(ctx context.Context, ids []int64) ([]*models.User, error) {
	if len(ids) == 0 {
		return []*models.User{}, nil
	}

	// Строим запрос с параметрами
	query := `
		SELECT id, username, first_name, last_name, created_at, updated_at
		FROM users
		WHERE id = ANY($1)
		ORDER BY id
	`

	rows, err := r.db.QueryContext(ctx, query, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get users by IDs: %w", err)
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		var user models.User
		err := rows.Scan(
			&user.ID, &user.Username, &user.FirstName, &user.LastName,
			&user.CreatedAt, &user.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}
		users = append(users, &user)
	}

	return users, nil
}

// List получает список пользователей
func (r *postgresRepository) List(ctx context.Context, limit, offset int) ([]*models.User, error) {
	query := `
		SELECT id, username, first_name, last_name, created_at, updated_at
		FROM users
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		var user models.User
		err := rows.Scan(
			&user.ID, &user.Username, &user.FirstName, &user.LastName,
			&user.CreatedAt, &user.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}
		users = append(users, &user)
	}

	return users, nil
}

// UpdateStatus обновляет статус пользователя
func (r *postgresRepository) UpdateStatus(ctx context.Context, id int64, status string) error {
	query := "UPDATE users SET status = $2, updated_at = NOW() WHERE id = $1"

	result, err := r.db.ExecContext(ctx, query, id, status)
	if err != nil {
		return fmt.Errorf("failed to update user status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repository.ErrUserNotFound
	}

	return nil
}

// GetUserStats получает статистику пользователя
func (r *postgresRepository) GetUserStats(ctx context.Context, userID int64) (*models.UserStats, error) {
	// Получаем количество созданных гивов
	var createdCount int
	err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM giveaways WHERE creator_id = $1", userID).Scan(&createdCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get created giveaways count: %w", err)
	}

	// Получаем количество участий
	var participatedCount int
	err = r.db.QueryRowContext(ctx,
		"SELECT COUNT(DISTINCT giveaway_id) FROM participants WHERE user_id = $1", userID).Scan(&participatedCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get participated giveaways count: %w", err)
	}

	// Получаем количество побед
	var winsCount int
	err = r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM win_records WHERE user_id = $1", userID).Scan(&winsCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get wins count: %w", err)
	}

	return &models.UserStats{
		CreatedGiveaways:  createdCount,
		ParticipatedCount: participatedCount,
		WinsCount:         winsCount,
	}, nil
}

// GetUserGiveaways получает гивы пользователя
func (r *postgresRepository) GetUserGiveaways(ctx context.Context, userID int64, status string) ([]*models.Giveaway, error) {
	query := `
		SELECT g.id, g.creator_id, g.title, g.description, g.started_at, g.ends_at, 
			g.duration, g.max_participants, g.winners_count, g.status, g.auto_distribute, 
			g.allow_tickets, g.msg_id, g.created_at, g.updated_at
		FROM giveaways g
		WHERE g.creator_id = $1
	`

	args := []interface{}{userID}
	argIndex := 2

	if status != "" {
		query += fmt.Sprintf(" AND g.status = $%d", argIndex)
		args = append(args, status)
		argIndex++
	}

	query += " ORDER BY g.created_at DESC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get user giveaways: %w", err)
	}
	defer rows.Close()

	var giveaways []*models.Giveaway
	for rows.Next() {
		var giveaway models.Giveaway
		err := rows.Scan(
			&giveaway.ID, &giveaway.CreatorID, &giveaway.Title, &giveaway.Description,
			&giveaway.StartedAt, &giveaway.EndsAt, &giveaway.Duration, &giveaway.MaxParticipants,
			&giveaway.WinnersCount, &giveaway.Status, &giveaway.AutoDistribute, &giveaway.AllowTickets,
			&giveaway.MsgID, &giveaway.CreatedAt, &giveaway.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan giveaway: %w", err)
		}
		giveaways = append(giveaways, &giveaway)
	}

	return giveaways, nil
}

// GetUserWins получает победы пользователя
func (r *postgresRepository) GetUserWins(ctx context.Context, userID int64) ([]*models.WinRecord, error) {
	query := `
		SELECT wr.id, wr.giveaway_id, wr.user_id, wr.prize_id, wr.place, wr.status, 
			wr.created_at, wr.updated_at, wr.received_at,
			g.title as giveaway_title,
			p.name as prize_name, p.description as prize_description
		FROM win_records wr
		JOIN giveaways g ON wr.giveaway_id = g.id
		LEFT JOIN prizes p ON wr.prize_id = p.id
		WHERE wr.user_id = $1
		ORDER BY wr.created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user wins: %w", err)
	}
	defer rows.Close()

	var wins []*models.WinRecord
	for rows.Next() {
		var win models.WinRecord
		var giveawayTitle, prizeName, prizeDescription sql.NullString
		var receivedAt sql.NullTime

		err := rows.Scan(
			&win.ID, &win.GiveawayID, &win.UserID, &win.PrizeID, &win.Place, &win.Status,
			&win.CreatedAt, &win.UpdatedAt, &receivedAt,
			&giveawayTitle, &prizeName, &prizeDescription)
		if err != nil {
			return nil, fmt.Errorf("failed to scan win record: %w", err)
		}

		if receivedAt.Valid {
			win.ReceivedAt = &receivedAt.Time
		}

		wins = append(wins, &win)
	}

	return wins, nil
}
