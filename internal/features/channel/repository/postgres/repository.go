package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"giveaway-tool-backend/internal/features/channel/models"
	"giveaway-tool-backend/internal/features/channel/repository"

	_ "github.com/lib/pq"
)

type postgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) repository.ChannelRepository {
	return &postgresRepository{db: db}
}

// SaveChannelInfo сохраняет информацию о канале
func (r *postgresRepository) SaveChannelInfo(ctx context.Context, channelInfo *models.ChannelInfo) error {
	query := `
		INSERT INTO sponsors (id, username, title, avatar_url, channel_url)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO UPDATE SET
			username = EXCLUDED.username,
			title = EXCLUDED.title,
			avatar_url = EXCLUDED.avatar_url,
			channel_url = EXCLUDED.channel_url
	`

	_, err := r.db.ExecContext(ctx, query,
		channelInfo.ID, channelInfo.Username, channelInfo.Title,
		channelInfo.AvatarURL, channelInfo.ChannelURL)
	if err != nil {
		return fmt.Errorf("failed to save channel info: %w", err)
	}

	return nil
}

// GetChannelInfo получает информацию о канале
func (r *postgresRepository) GetChannelInfo(ctx context.Context, channelID int64) (*models.ChannelInfo, error) {
	query := `
		SELECT id, username, title, avatar_url, channel_url, created_at
		FROM sponsors
		WHERE id = $1
	`

	var channel models.ChannelInfo
	err := r.db.QueryRowContext(ctx, query, channelID).Scan(
		&channel.ID, &channel.Username, &channel.Title,
		&channel.AvatarURL, &channel.ChannelURL, &channel.CreatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, repository.ErrChannelNotFound
		}
		return nil, fmt.Errorf("failed to get channel info: %w", err)
	}

	return &channel, nil
}

// GetChannelInfoByID получает информацию о канале по ID (алиас для GetChannelInfo)
func (r *postgresRepository) GetChannelInfoByID(ctx context.Context, channelID int64) (*models.ChannelInfo, error) {
	return r.GetChannelInfo(ctx, channelID)
}

// GetChannelInfoByUsername получает информацию о канале по username
func (r *postgresRepository) GetChannelInfoByUsername(ctx context.Context, username string) (*models.ChannelInfo, error) {
	query := `
		SELECT id, username, title, avatar_url, channel_url, created_at
		FROM sponsors
		WHERE username = $1
	`

	var channel models.ChannelInfo
	err := r.db.QueryRowContext(ctx, query, username).Scan(
		&channel.ID, &channel.Username, &channel.Title,
		&channel.AvatarURL, &channel.ChannelURL, &channel.CreatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, repository.ErrChannelNotFound
		}
		return nil, fmt.Errorf("failed to get channel info by username: %w", err)
	}

	return &channel, nil
}

// SetChannelAvatar сохраняет URL аватара канала
func (r *postgresRepository) SetChannelAvatar(ctx context.Context, username string, avatarURL string) error {
	query := `
		UPDATE sponsors 
		SET avatar_url = $2
		WHERE username = $1
	`

	result, err := r.db.ExecContext(ctx, query, username, avatarURL)
	if err != nil {
		return fmt.Errorf("failed to set channel avatar: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repository.ErrChannelNotFound
	}

	return nil
}

// GetChannelTitle получает название канала
func (r *postgresRepository) GetChannelTitle(ctx context.Context, chatID int64) (string, error) {
	query := "SELECT title FROM sponsors WHERE id = $1"

	var title string
	err := r.db.QueryRowContext(ctx, query, chatID).Scan(&title)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", repository.ErrChannelNotFound
		}
		return "", fmt.Errorf("failed to get channel title: %w", err)
	}

	return title, nil
}

// GetChannelUsername получает username канала
func (r *postgresRepository) GetChannelUsername(ctx context.Context, channelID int64) (string, error) {
	query := "SELECT username FROM sponsors WHERE id = $1"

	var username string
	err := r.db.QueryRowContext(ctx, query, channelID).Scan(&username)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", repository.ErrChannelNotFound
		}
		return "", fmt.Errorf("failed to get channel username: %w", err)
	}

	return username, nil
}

// GetChannelsByIDs получает каналы по списку ID
func (r *postgresRepository) GetChannelsByIDs(ctx context.Context, ids []int64) ([]*models.ChannelInfo, error) {
	if len(ids) == 0 {
		return []*models.ChannelInfo{}, nil
	}

	query := `
		SELECT id, username, title, avatar_url, channel_url, created_at
		FROM sponsors
		WHERE id = ANY($1)
		ORDER BY id
	`

	rows, err := r.db.QueryContext(ctx, query, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get channels by IDs: %w", err)
	}
	defer rows.Close()

	var channels []*models.ChannelInfo
	for rows.Next() {
		var channel models.ChannelInfo
		err := rows.Scan(
			&channel.ID, &channel.Username, &channel.Title,
			&channel.AvatarURL, &channel.ChannelURL, &channel.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan channel: %w", err)
		}
		channels = append(channels, &channel)
	}

	return channels, nil
}

// Create создает новый канал
func (r *postgresRepository) Create(ctx context.Context, channel *models.Channel) error {
	query := `
		INSERT INTO channels (id, username, title, avatar_url, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err := r.db.ExecContext(ctx, query,
		channel.ID, channel.Username, channel.Title, channel.AvatarURL,
		channel.Status, channel.CreatedAt, channel.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to create channel: %w", err)
	}

	return nil
}

// GetByID получает канал по ID
func (r *postgresRepository) GetByID(ctx context.Context, id int64) (*models.Channel, error) {
	query := `
		SELECT id, username, title, avatar_url, status, created_at, updated_at
		FROM channels
		WHERE id = $1
	`

	var channel models.Channel
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&channel.ID, &channel.Username, &channel.Title, &channel.AvatarURL,
		&channel.Status, &channel.CreatedAt, &channel.UpdatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, repository.ErrChannelNotFound
		}
		return nil, fmt.Errorf("failed to get channel by ID: %w", err)
	}

	return &channel, nil
}

// Update обновляет канал
func (r *postgresRepository) Update(ctx context.Context, channel *models.Channel) error {
	query := `
		UPDATE channels
		SET username = $2, title = $3, avatar_url = $4, status = $5, updated_at = $6
		WHERE id = $1
	`

	result, err := r.db.ExecContext(ctx, query,
		channel.ID, channel.Username, channel.Title, channel.AvatarURL,
		channel.Status, channel.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to update channel: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repository.ErrChannelNotFound
	}

	return nil
}

// Delete удаляет канал
func (r *postgresRepository) Delete(ctx context.Context, id int64) error {
	query := "DELETE FROM channels WHERE id = $1"

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete channel: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return repository.ErrChannelNotFound
	}

	return nil
}

// List получает список каналов с пагинацией
func (r *postgresRepository) List(ctx context.Context, offset, limit int) ([]*models.Channel, error) {
	query := `
		SELECT id, username, title, avatar_url, status, created_at, updated_at
		FROM channels
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list channels: %w", err)
	}
	defer rows.Close()

	var channels []*models.Channel
	for rows.Next() {
		var channel models.Channel
		err := rows.Scan(
			&channel.ID, &channel.Username, &channel.Title, &channel.AvatarURL,
			&channel.Status, &channel.CreatedAt, &channel.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan channel: %w", err)
		}
		channels = append(channels, &channel)
	}

	return channels, nil
}

// GetChannelStats получает статистику канала
func (r *postgresRepository) GetChannelStats(ctx context.Context, channelID int64) (*models.ChannelStats, error) {
	query := `
		SELECT 
			$1 as channel_id,
			COUNT(DISTINCT g.id) as total_giveaways,
			COUNT(DISTINCT CASE WHEN g.status = 'active' THEN g.id END) as active_giveaways,
			COUNT(DISTINCT p.user_id) as total_participants,
			COUNT(DISTINCT w.user_id) as total_winners
		FROM channels c
		LEFT JOIN giveaways g ON g.creator_id = c.id
		LEFT JOIN participants p ON p.giveaway_id = g.id
		LEFT JOIN winners w ON w.giveaway_id = g.id
		WHERE c.id = $1
	`

	var stats models.ChannelStats
	err := r.db.QueryRowContext(ctx, query, channelID).Scan(
		&stats.ChannelID, &stats.TotalGiveaways, &stats.ActiveGiveaways,
		&stats.TotalParticipants, &stats.TotalWinners)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, repository.ErrChannelNotFound
		}
		return nil, fmt.Errorf("failed to get channel stats: %w", err)
	}

	return &stats, nil
}
