package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"giveaway-tool-backend/internal/features/giveaway/repository"
)

type postgresTicketRepository struct {
	db *sql.DB
}

func NewPostgresTicketRepository(db *sql.DB) repository.TicketRepository {
	return &postgresTicketRepository{db: db}
}

// AddTickets добавляет билеты пользователю
func (r *postgresTicketRepository) AddTickets(ctx context.Context, giveawayID string, userID int64, count int) error {
	query := `
		INSERT INTO tickets (giveaway_id, user_id, count)
		VALUES ($1, $2, $3)
		ON CONFLICT (giveaway_id, user_id) DO UPDATE SET
			count = tickets.count + EXCLUDED.count,
			updated_at = NOW()
	`

	_, err := r.db.ExecContext(ctx, query, giveawayID, userID, count)
	if err != nil {
		return fmt.Errorf("failed to add tickets: %w", err)
	}

	return nil
}

// GetTickets получает количество билетов пользователя
func (r *postgresTicketRepository) GetTickets(ctx context.Context, giveawayID string, userID int64) (int, error) {
	query := "SELECT count FROM tickets WHERE giveaway_id = $1 AND user_id = $2"

	var count int
	err := r.db.QueryRowContext(ctx, query, giveawayID, userID).Scan(&count)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil // Пользователь не имеет билетов
		}
		return 0, fmt.Errorf("failed to get tickets: %w", err)
	}

	return count, nil
}

// GetAllTickets получает все билеты для гива
func (r *postgresTicketRepository) GetAllTickets(ctx context.Context, giveawayID string) (map[int64]int, error) {
	query := "SELECT user_id, count FROM tickets WHERE giveaway_id = $1"

	rows, err := r.db.QueryContext(ctx, query, giveawayID)
	if err != nil {
		return nil, fmt.Errorf("failed to get all tickets: %w", err)
	}
	defer rows.Close()

	tickets := make(map[int64]int)
	for rows.Next() {
		var userID int64
		var count int
		err := rows.Scan(&userID, &count)
		if err != nil {
			return nil, fmt.Errorf("failed to scan ticket: %w", err)
		}
		tickets[userID] = count
	}

	return tickets, nil
}

// GetTotalTickets получает общее количество билетов для гива
func (r *postgresTicketRepository) GetTotalTickets(ctx context.Context, giveawayID string) (int, error) {
	query := "SELECT COALESCE(SUM(count), 0) FROM tickets WHERE giveaway_id = $1"

	var total int
	err := r.db.QueryRowContext(ctx, query, giveawayID).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("failed to get total tickets: %w", err)
	}

	return total, nil
}

// GetUserTickets получает билеты пользователя для всех гивов
func (r *postgresTicketRepository) GetUserTickets(ctx context.Context, userID int64) (map[string]int, error) {
	query := "SELECT giveaway_id, count FROM tickets WHERE user_id = $1"

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user tickets: %w", err)
	}
	defer rows.Close()

	tickets := make(map[string]int)
	for rows.Next() {
		var giveawayID string
		var count int
		err := rows.Scan(&giveawayID, &count)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user ticket: %w", err)
		}
		tickets[giveawayID] = count
	}

	return tickets, nil
}

// DeleteTickets удаляет билеты пользователя
func (r *postgresTicketRepository) DeleteTickets(ctx context.Context, giveawayID string, userID int64) error {
	query := "DELETE FROM tickets WHERE giveaway_id = $1 AND user_id = $2"

	_, err := r.db.ExecContext(ctx, query, giveawayID, userID)
	if err != nil {
		return fmt.Errorf("failed to delete tickets: %w", err)
	}

	return nil
}
