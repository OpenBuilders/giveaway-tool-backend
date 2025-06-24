package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"giveaway-tool-backend/internal/features/giveaway/models"
	"giveaway-tool-backend/internal/features/giveaway/repository"
	usermodels "giveaway-tool-backend/internal/features/user/models"
	"time"

	_ "github.com/lib/pq"
)

type postgresRepository struct {
	db *sql.DB
}

type postgresTransaction struct {
	tx *sql.Tx
}

func (t *postgresTransaction) Commit() error {
	return t.tx.Commit()
}

func (t *postgresTransaction) Rollback() error {
	return t.tx.Rollback()
}

func NewPostgresRepository(db *sql.DB) repository.GiveawayRepository {
	return &postgresRepository{db: db}
}

func (r *postgresRepository) BeginTx(ctx context.Context) (repository.Transaction, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	return &postgresTransaction{tx: tx}, nil
}

func (r *postgresRepository) Create(ctx context.Context, giveaway *models.Giveaway) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	query := `
		INSERT INTO giveaways (id, creator_id, title, description, started_at, ends_at, duration, 
			max_participants, winners_count, status, auto_distribute, allow_tickets, msg_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	_, err = tx.ExecContext(ctx, query,
		giveaway.ID, giveaway.CreatorID, giveaway.Title, giveaway.Description,
		giveaway.StartedAt, giveaway.EndsAt, giveaway.Duration, giveaway.MaxParticipants,
		giveaway.WinnersCount, giveaway.Status, giveaway.AutoDistribute, giveaway.AllowTickets, giveaway.MsgID)
	if err != nil {
		return fmt.Errorf("failed to create giveaway: %w", err)
	}

	for _, prize := range giveaway.Prizes {
		prizeQuery := `
			INSERT INTO prizes (id, type, name, description, is_internal)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (id) DO NOTHING
		`
		_, err = tx.ExecContext(ctx, prizeQuery,
			prize.PrizeID, prize.PrizeType, prize.PrizeID, "", prize.PrizeType == models.PrizeTypeInternal)
		if err != nil {
			return fmt.Errorf("failed to create prize: %w", err)
		}

		giveawayPrizeQuery := `
			INSERT INTO giveaway_prizes (giveaway_id, prize_id, place)
			VALUES ($1, $2, $3)
		`
		_, err = tx.ExecContext(ctx, giveawayPrizeQuery, giveaway.ID, prize.PrizeID, prize.GetPlace())
		if err != nil {
			return fmt.Errorf("failed to link prize to giveaway: %w", err)
		}
	}

	// Добавляем требования
	for _, req := range giveaway.Requirements {
		reqQuery := `
			INSERT INTO requirements (giveaway_id, type, username, description)
			VALUES ($1, $2, $3, $4)
		`
		_, err = tx.ExecContext(ctx, reqQuery, giveaway.ID, req.Type, req.Username, req.Description)
		if err != nil {
			return fmt.Errorf("failed to create requirement: %w", err)
		}
	}

	// Добавляем спонсоров
	for _, sponsor := range giveaway.Sponsors {
		// Сначала создаем спонсора, если его нет
		sponsorQuery := `
			INSERT INTO sponsors (id, username, title, avatar_url, channel_url)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (id) DO UPDATE SET
				username = EXCLUDED.username,
				title = EXCLUDED.title,
				avatar_url = EXCLUDED.avatar_url,
				channel_url = EXCLUDED.channel_url
		`
		_, err = tx.ExecContext(ctx, sponsorQuery,
			sponsor.ID, sponsor.Username, sponsor.Title, sponsor.AvatarURL, sponsor.ChannelURL)
		if err != nil {
			return fmt.Errorf("failed to create sponsor: %w", err)
		}

		// Связываем спонсора с гивом
		giveawaySponsorQuery := `
			INSERT INTO giveaway_sponsors (giveaway_id, sponsor_id)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`
		_, err = tx.ExecContext(ctx, giveawaySponsorQuery, giveaway.ID, sponsor.ID)
		if err != nil {
			return fmt.Errorf("failed to link sponsor to giveaway: %w", err)
		}
	}

	return tx.Commit()
}

// GetByID получает гив по ID
func (r *postgresRepository) GetByID(ctx context.Context, id string) (*models.Giveaway, error) {
	query := `
		SELECT g.id, g.creator_id, g.title, g.description, g.started_at, g.ends_at, 
			g.duration, g.max_participants, g.winners_count, g.status, g.auto_distribute, 
			g.allow_tickets, g.msg_id, g.created_at, g.updated_at
		FROM giveaways g
		WHERE g.id = $1
	`

	var giveaway models.Giveaway
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&giveaway.ID, &giveaway.CreatorID, &giveaway.Title, &giveaway.Description,
		&giveaway.StartedAt, &giveaway.EndsAt, &giveaway.Duration, &giveaway.MaxParticipants,
		&giveaway.WinnersCount, &giveaway.Status, &giveaway.AutoDistribute, &giveaway.AllowTickets,
		&giveaway.MsgID, &giveaway.CreatedAt, &giveaway.UpdatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, repository.ErrGiveawayNotFound
		}
		return nil, fmt.Errorf("failed to get giveaway: %w", err)
	}

	// Загружаем призы
	prizes, err := r.getPrizes(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get prizes: %w", err)
	}
	giveaway.Prizes = prizes

	// Загружаем требования
	requirements, err := r.getRequirements(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get requirements: %w", err)
	}
	giveaway.Requirements = requirements

	// Загружаем спонсоров
	sponsors, err := r.getSponsors(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get sponsors: %w", err)
	}
	giveaway.Sponsors = sponsors

	return &giveaway, nil
}

// GetByIDWithLock получает гив по ID с блокировкой
func (r *postgresRepository) GetByIDWithLock(ctx context.Context, tx repository.Transaction, id string) (*models.Giveaway, error) {
	postgresTx, ok := tx.(*postgresTransaction)
	if !ok {
		return nil, fmt.Errorf("invalid transaction type")
	}

	query := `
		SELECT g.id, g.creator_id, g.title, g.description, g.started_at, g.ends_at, 
			g.duration, g.max_participants, g.winners_count, g.status, g.auto_distribute, 
			g.allow_tickets, g.msg_id, g.created_at, g.updated_at
		FROM giveaways g
		WHERE g.id = $1
		FOR UPDATE
	`

	var giveaway models.Giveaway
	err := postgresTx.tx.QueryRowContext(ctx, query, id).Scan(
		&giveaway.ID, &giveaway.CreatorID, &giveaway.Title, &giveaway.Description,
		&giveaway.StartedAt, &giveaway.EndsAt, &giveaway.Duration, &giveaway.MaxParticipants,
		&giveaway.WinnersCount, &giveaway.Status, &giveaway.AutoDistribute, &giveaway.AllowTickets,
		&giveaway.MsgID, &giveaway.CreatedAt, &giveaway.UpdatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, repository.ErrGiveawayNotFound
		}
		return nil, fmt.Errorf("failed to get giveaway: %w", err)
	}

	prizes, err := r.getPrizesTx(ctx, postgresTx.tx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get prizes: %w", err)
	}
	giveaway.Prizes = prizes

	requirements, err := r.getRequirementsTx(ctx, postgresTx.tx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get requirements: %w", err)
	}
	giveaway.Requirements = requirements

	sponsors, err := r.getSponsorsTx(ctx, postgresTx.tx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get sponsors: %w", err)
	}
	giveaway.Sponsors = sponsors

	return &giveaway, nil
}

// Update обновляет гив
func (r *postgresRepository) Update(ctx context.Context, giveaway *models.Giveaway) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	query := `
		UPDATE giveaways 
		SET title = $2, description = $3, status = $4, updated_at = NOW()
		WHERE id = $1
	`
	_, err = tx.ExecContext(ctx, query, giveaway.ID, giveaway.Title, giveaway.Description, giveaway.Status)
	if err != nil {
		return fmt.Errorf("failed to update giveaway: %w", err)
	}

	_, err = tx.ExecContext(ctx, "DELETE FROM giveaway_prizes WHERE giveaway_id = $1", giveaway.ID)
	if err != nil {
		return fmt.Errorf("failed to delete old prizes: %w", err)
	}

	for _, prize := range giveaway.Prizes {
		prizeQuery := `
			INSERT INTO prizes (id, type, name, description, is_internal)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (id) DO NOTHING
		`
		_, err = tx.ExecContext(ctx, prizeQuery,
			prize.PrizeID, prize.PrizeType, prize.PrizeID, "", prize.PrizeType == models.PrizeTypeInternal)
		if err != nil {
			return fmt.Errorf("failed to create prize: %w", err)
		}

		giveawayPrizeQuery := `
			INSERT INTO giveaway_prizes (giveaway_id, prize_id, place)
			VALUES ($1, $2, $3)
		`
		_, err = tx.ExecContext(ctx, giveawayPrizeQuery, giveaway.ID, prize.PrizeID, prize.GetPlace())
		if err != nil {
			return fmt.Errorf("failed to link prize to giveaway: %w", err)
		}
	}

	_, err = tx.ExecContext(ctx, "DELETE FROM giveaway_sponsors WHERE giveaway_id = $1", giveaway.ID)
	if err != nil {
		return fmt.Errorf("failed to delete old sponsors: %w", err)
	}

	for _, sponsor := range giveaway.Sponsors {
		sponsorQuery := `
			INSERT INTO sponsors (id, username, title, avatar_url, channel_url)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (id) DO UPDATE SET
				username = EXCLUDED.username,
				title = EXCLUDED.title,
				avatar_url = EXCLUDED.avatar_url,
				channel_url = EXCLUDED.channel_url
		`
		_, err = tx.ExecContext(ctx, sponsorQuery,
			sponsor.ID, sponsor.Username, sponsor.Title, sponsor.AvatarURL, sponsor.ChannelURL)
		if err != nil {
			return fmt.Errorf("failed to create sponsor: %w", err)
		}

		giveawaySponsorQuery := `
			INSERT INTO giveaway_sponsors (giveaway_id, sponsor_id)
			VALUES ($1, $2)
		`
		_, err = tx.ExecContext(ctx, giveawaySponsorQuery, giveaway.ID, sponsor.ID)
		if err != nil {
			return fmt.Errorf("failed to link sponsor to giveaway: %w", err)
		}
	}

	return tx.Commit()
}

func (r *postgresRepository) UpdateTx(ctx context.Context, tx repository.Transaction, giveaway *models.Giveaway) error {
	postgresTx, ok := tx.(*postgresTransaction)
	if !ok {
		return fmt.Errorf("invalid transaction type")
	}

	query := `
		UPDATE giveaways 
		SET title = $2, description = $3, status = $4, updated_at = NOW()
		WHERE id = $1
	`
	_, err := postgresTx.tx.ExecContext(ctx, query, giveaway.ID, giveaway.Title, giveaway.Description, giveaway.Status)
	if err != nil {
		return fmt.Errorf("failed to update giveaway: %w", err)
	}

	return nil
}

func (r *postgresRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM giveaways WHERE id = $1", id)
	return err
}

func (r *postgresRepository) AddParticipant(ctx context.Context, giveawayID string, userID int64) error {
	count, err := r.GetParticipantsCount(ctx, giveawayID)
	if err != nil {
		return fmt.Errorf("failed to get participants count: %w", err)
	}

	giveaway, err := r.GetByID(ctx, giveawayID)
	if err != nil {
		return fmt.Errorf("failed to get giveaway: %w", err)
	}

	if giveaway.Status != models.GiveawayStatusActive {
		return fmt.Errorf("cannot join giveaway with status: %s", giveaway.Status)
	}

	if giveaway.MaxParticipants > 0 && count >= int64(giveaway.MaxParticipants) {
		return fmt.Errorf("max participants reached")
	}

	query := `
		INSERT INTO participants (giveaway_id, user_id)
		VALUES ($1, $2)
		ON CONFLICT (giveaway_id, user_id) DO NOTHING
	`

	_, err = r.db.ExecContext(ctx, query, giveawayID, userID)
	if err != nil {
		return fmt.Errorf("failed to add participant: %w", err)
	}

	return nil
}

// GetParticipants получает список участников
func (r *postgresRepository) GetParticipants(ctx context.Context, giveawayID string) ([]int64, error) {
	query := "SELECT user_id FROM participants WHERE giveaway_id = $1 ORDER BY joined_at"

	rows, err := r.db.QueryContext(ctx, query, giveawayID)
	if err != nil {
		return nil, fmt.Errorf("failed to get participants: %w", err)
	}
	defer rows.Close()

	var participants []int64
	for rows.Next() {
		var userID int64
		if err := rows.Scan(&userID); err != nil {
			return nil, fmt.Errorf("failed to scan participant: %w", err)
		}
		participants = append(participants, userID)
	}

	return participants, nil
}

// GetParticipantsTx получает список участников в транзакции
func (r *postgresRepository) GetParticipantsTx(ctx context.Context, tx repository.Transaction, giveawayID string) ([]int64, error) {
	postgresTx, ok := tx.(*postgresTransaction)
	if !ok {
		return nil, fmt.Errorf("invalid transaction type")
	}

	query := "SELECT user_id FROM participants WHERE giveaway_id = $1 ORDER BY joined_at"

	rows, err := postgresTx.tx.QueryContext(ctx, query, giveawayID)
	if err != nil {
		return nil, fmt.Errorf("failed to get participants: %w", err)
	}
	defer rows.Close()

	var participants []int64
	for rows.Next() {
		var userID int64
		if err := rows.Scan(&userID); err != nil {
			return nil, fmt.Errorf("failed to scan participant: %w", err)
		}
		participants = append(participants, userID)
	}

	return participants, nil
}

// GetParticipantsCount получает количество участников
func (r *postgresRepository) GetParticipantsCount(ctx context.Context, giveawayID string) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM participants WHERE giveaway_id = $1", giveawayID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get participants count: %w", err)
	}
	return count, nil
}

// IsParticipant проверяет, является ли пользователь участником
func (r *postgresRepository) IsParticipant(ctx context.Context, giveawayID string, userID int64) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM participants WHERE giveaway_id = $1 AND user_id = $2)",
		giveawayID, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check participant: %w", err)
	}
	return exists, nil
}

// CreateWinRecord создает запись о победе
func (r *postgresRepository) CreateWinRecord(ctx context.Context, record *models.WinRecord) error {
	query := `
		INSERT INTO win_records (id, giveaway_id, user_id, prize_id, place, status)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := r.db.ExecContext(ctx, query,
		record.ID, record.GiveawayID, record.UserID, record.PrizeID, record.Place, record.Status)
	if err != nil {
		return fmt.Errorf("failed to create win record: %w", err)
	}
	return nil
}

// CreateWinRecordTx создает запись о победе в транзакции
func (r *postgresRepository) CreateWinRecordTx(ctx context.Context, tx repository.Transaction, record *models.WinRecord) error {
	postgresTx, ok := tx.(*postgresTransaction)
	if !ok {
		return fmt.Errorf("invalid transaction type")
	}

	query := `
		INSERT INTO win_records (id, giveaway_id, user_id, prize_id, place, status)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := postgresTx.tx.ExecContext(ctx, query,
		record.ID, record.GiveawayID, record.UserID, record.PrizeID, record.Place, record.Status)
	if err != nil {
		return fmt.Errorf("failed to create win record: %w", err)
	}
	return nil
}

// SavePreWinnerList сохраняет pre-winner list
func (r *postgresRepository) SavePreWinnerList(ctx context.Context, giveawayID string, preWinnerList *models.PreWinnerListStored) error {
	// Удаляем старые данные
	_, err := r.db.ExecContext(ctx, "DELETE FROM pre_winner_lists WHERE giveaway_id = $1", giveawayID)
	if err != nil {
		return fmt.Errorf("failed to delete old pre-winner list: %w", err)
	}

	// Сохраняем новые данные
	query := `
		INSERT INTO pre_winner_lists (giveaway_id, user_ids, expires_at)
		VALUES ($1, $2, NOW() + INTERVAL '24 hours')
	`
	_, err = r.db.ExecContext(ctx, query, giveawayID, preWinnerList.UserIDs)
	if err != nil {
		return fmt.Errorf("failed to save pre-winner list: %w", err)
	}

	return nil
}

// GetPreWinnerList получает pre-winner list
func (r *postgresRepository) GetPreWinnerList(ctx context.Context, giveawayID string) (*models.PreWinnerListStored, error) {
	query := "SELECT user_ids, created_at FROM pre_winner_lists WHERE giveaway_id = $1 AND expires_at > NOW()"

	var userIDs []int64
	var createdAt time.Time

	err := r.db.QueryRowContext(ctx, query, giveawayID).Scan(&userIDs, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, repository.ErrGiveawayNotFound
		}
		return nil, fmt.Errorf("failed to get pre-winner list: %w", err)
	}

	return &models.PreWinnerListStored{
		GiveawayID: giveawayID,
		UserIDs:    userIDs,
		CreatedAt:  createdAt.Unix(),
	}, nil
}

// DeletePreWinnerList удаляет pre-winner list
func (r *postgresRepository) DeletePreWinnerList(ctx context.Context, giveawayID string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM pre_winner_lists WHERE giveaway_id = $1", giveawayID)
	return err
}

// Вспомогательные методы
func (r *postgresRepository) getPrizes(ctx context.Context, giveawayID string) ([]models.PrizePlace, error) {
	query := `
		SELECT p.id, p.type, gp.place
		FROM prizes p
		JOIN giveaway_prizes gp ON p.id = gp.prize_id
		WHERE gp.giveaway_id = $1
		ORDER BY gp.place
	`

	rows, err := r.db.QueryContext(ctx, query, giveawayID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prizes []models.PrizePlace
	for rows.Next() {
		var prize models.PrizePlace
		var place int
		err := rows.Scan(&prize.PrizeID, &prize.PrizeType, &place)
		if err != nil {
			return nil, err
		}
		prize.Place = place
		prizes = append(prizes, prize)
	}

	return prizes, nil
}

func (r *postgresRepository) getPrizesTx(ctx context.Context, tx *sql.Tx, giveawayID string) ([]models.PrizePlace, error) {
	query := `
		SELECT p.id, p.type, gp.place
		FROM prizes p
		JOIN giveaway_prizes gp ON p.id = gp.prize_id
		WHERE gp.giveaway_id = $1
		ORDER BY gp.place
	`

	rows, err := tx.QueryContext(ctx, query, giveawayID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prizes []models.PrizePlace
	for rows.Next() {
		var prize models.PrizePlace
		var place int
		err := rows.Scan(&prize.PrizeID, &prize.PrizeType, &place)
		if err != nil {
			return nil, err
		}
		prize.Place = place
		prizes = append(prizes, prize)
	}

	return prizes, nil
}

func (r *postgresRepository) getRequirements(ctx context.Context, giveawayID string) ([]models.Requirement, error) {
	query := "SELECT type, username, description FROM requirements WHERE giveaway_id = $1 ORDER BY id"

	rows, err := r.db.QueryContext(ctx, query, giveawayID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var requirements []models.Requirement
	for rows.Next() {
		var req models.Requirement
		err := rows.Scan(&req.Type, &req.Username, &req.Description)
		if err != nil {
			return nil, err
		}
		requirements = append(requirements, req)
	}

	return requirements, nil
}

func (r *postgresRepository) getRequirementsTx(ctx context.Context, tx *sql.Tx, giveawayID string) ([]models.Requirement, error) {
	query := "SELECT type, username, description FROM requirements WHERE giveaway_id = $1 ORDER BY id"

	rows, err := tx.QueryContext(ctx, query, giveawayID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var requirements []models.Requirement
	for rows.Next() {
		var req models.Requirement
		err := rows.Scan(&req.Type, &req.Username, &req.Description)
		if err != nil {
			return nil, err
		}
		requirements = append(requirements, req)
	}

	return requirements, nil
}

func (r *postgresRepository) getSponsors(ctx context.Context, giveawayID string) ([]models.ChannelInfo, error) {
	query := `
		SELECT s.id, s.username, s.title, s.avatar_url, s.channel_url
		FROM sponsors s
		JOIN giveaway_sponsors gs ON s.id = gs.sponsor_id
		WHERE gs.giveaway_id = $1
		ORDER BY gs.created_at
	`

	rows, err := r.db.QueryContext(ctx, query, giveawayID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sponsors []models.ChannelInfo
	for rows.Next() {
		var sponsor models.ChannelInfo
		err := rows.Scan(&sponsor.ID, &sponsor.Username, &sponsor.Title, &sponsor.AvatarURL, &sponsor.ChannelURL)
		if err != nil {
			return nil, err
		}
		sponsors = append(sponsors, sponsor)
	}

	return sponsors, nil
}

func (r *postgresRepository) getSponsorsTx(ctx context.Context, tx *sql.Tx, giveawayID string) ([]models.ChannelInfo, error) {
	query := `
		SELECT s.id, s.username, s.title, s.avatar_url, s.channel_url
		FROM sponsors s
		JOIN giveaway_sponsors gs ON s.id = gs.sponsor_id
		WHERE gs.giveaway_id = $1
		ORDER BY gs.created_at
	`

	rows, err := tx.QueryContext(ctx, query, giveawayID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sponsors []models.ChannelInfo
	for rows.Next() {
		var sponsor models.ChannelInfo
		err := rows.Scan(&sponsor.ID, &sponsor.Username, &sponsor.Title, &sponsor.AvatarURL, &sponsor.ChannelURL)
		if err != nil {
			return nil, err
		}
		sponsors = append(sponsors, sponsor)
	}

	return sponsors, nil
}

// Заглушки для методов, которые нужно реализовать позже
func (r *postgresRepository) GetActiveGiveaways(ctx context.Context) ([]string, error) {
	query := "SELECT id FROM giveaways WHERE status = 'active' ORDER BY created_at DESC"
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (r *postgresRepository) GetPendingGiveaways(ctx context.Context) ([]*models.Giveaway, error) {
	query := "SELECT id FROM giveaways WHERE status = 'pending' ORDER BY created_at DESC"
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var giveaways []*models.Giveaway
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		giveaway, err := r.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		giveaways = append(giveaways, giveaway)
	}
	return giveaways, nil
}

func (r *postgresRepository) AddToPending(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE giveaways SET status = 'pending' WHERE id = $1", id)
	return err
}

func (r *postgresRepository) AddToPendingTx(ctx context.Context, tx repository.Transaction, id string) error {
	postgresTx, ok := tx.(*postgresTransaction)
	if !ok {
		return fmt.Errorf("invalid transaction type")
	}
	_, err := postgresTx.tx.ExecContext(ctx, "UPDATE giveaways SET status = 'pending' WHERE id = $1", id)
	return err
}

func (r *postgresRepository) AddToHistory(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE giveaways SET status = 'history' WHERE id = $1", id)
	return err
}

func (r *postgresRepository) AddToHistoryTx(ctx context.Context, tx repository.Transaction, id string) error {
	postgresTx, ok := tx.(*postgresTransaction)
	if !ok {
		return fmt.Errorf("invalid transaction type")
	}
	_, err := postgresTx.tx.ExecContext(ctx, "UPDATE giveaways SET status = 'history' WHERE id = $1", id)
	return err
}

func (r *postgresRepository) MoveToHistory(ctx context.Context, id string) error {
	return r.AddToHistory(ctx, id)
}

func (r *postgresRepository) UpdateStatusAtomic(ctx context.Context, tx repository.Transaction, id string, update models.GiveawayStatusUpdate) error {
	postgresTx, ok := tx.(*postgresTransaction)
	if !ok {
		return fmt.Errorf("invalid transaction type")
	}
	_, err := postgresTx.tx.ExecContext(ctx,
		"UPDATE giveaways SET status = $2, updated_at = NOW() WHERE id = $1",
		id, update.NewStatus)
	return err
}

func (r *postgresRepository) UpdateStatus(ctx context.Context, id string, status models.GiveawayStatus) error {
	_, err := r.db.ExecContext(ctx, "UPDATE giveaways SET status = $2, updated_at = NOW() WHERE id = $1", id, status)
	return err
}

func (r *postgresRepository) UpdateStatusIfPending(ctx context.Context, id string, status models.GiveawayStatus) (bool, error) {
	result, err := r.db.ExecContext(ctx,
		"UPDATE giveaways SET status = $2, updated_at = NOW() WHERE id = $1 AND status = 'pending'",
		id, status)
	if err != nil {
		return false, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rowsAffected > 0, nil
}

func (r *postgresRepository) SelectWinners(ctx context.Context, giveawayID string, count int) ([]models.Winner, error) {
	// Простая реализация - выбираем случайных участников
	query := `
		SELECT user_id FROM participants 
		WHERE giveaway_id = $1 
		ORDER BY RANDOM() 
		LIMIT $2
	`
	rows, err := r.db.QueryContext(ctx, query, giveawayID, count)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var winners []models.Winner
	for i := 0; rows.Next(); i++ {
		var userID int64
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		winners = append(winners, models.Winner{
			UserID: userID,
			Place:  i + 1,
		})
	}
	return winners, nil
}

func (r *postgresRepository) SelectWinnersTx(ctx context.Context, tx repository.Transaction, giveawayID string, count int) ([]models.Winner, error) {
	postgresTx, ok := tx.(*postgresTransaction)
	if !ok {
		return nil, fmt.Errorf("invalid transaction type")
	}

	query := `
		SELECT user_id FROM participants 
		WHERE giveaway_id = $1 
		ORDER BY RANDOM() 
		LIMIT $2
	`
	rows, err := postgresTx.tx.QueryContext(ctx, query, giveawayID, count)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var winners []models.Winner
	for i := 0; rows.Next(); i++ {
		var userID int64
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		winners = append(winners, models.Winner{
			UserID: userID,
			Place:  i + 1,
		})
	}
	return winners, nil
}

func (r *postgresRepository) GetWinners(ctx context.Context, giveawayID string) ([]models.Winner, error) {
	query := `
		SELECT wr.user_id, wr.place
		FROM win_records wr
		WHERE wr.giveaway_id = $1
		ORDER BY wr.place
	`
	rows, err := r.db.QueryContext(ctx, query, giveawayID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var winners []models.Winner
	for rows.Next() {
		var winner models.Winner
		if err := rows.Scan(&winner.UserID, &winner.Place); err != nil {
			return nil, err
		}
		winners = append(winners, winner)
	}
	return winners, nil
}

// Остальные методы - заглушки
func (r *postgresRepository) CreatePrize(ctx context.Context, prize *models.Prize) error {
	return fmt.Errorf("not implemented")
}

func (r *postgresRepository) GetPrize(ctx context.Context, id string) (*models.Prize, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *postgresRepository) GetPrizeTx(ctx context.Context, tx repository.Transaction, id string) (*models.Prize, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *postgresRepository) GetPrizes(ctx context.Context, giveawayID string) ([]models.PrizePlace, error) {
	return r.getPrizes(ctx, giveawayID)
}

func (r *postgresRepository) GetPrizesTx(ctx context.Context, tx repository.Transaction, giveawayID string) ([]models.PrizePlace, error) {
	postgresTx, ok := tx.(*postgresTransaction)
	if !ok {
		return nil, fmt.Errorf("invalid transaction type")
	}
	return r.getPrizesTx(ctx, postgresTx.tx, giveawayID)
}

func (r *postgresRepository) AssignPrizeTx(ctx context.Context, tx repository.Transaction, userID int64, prizeID string, place int) error {
	return fmt.Errorf("not implemented")
}

func (r *postgresRepository) GetPrizeTemplates(ctx context.Context) ([]*models.PrizeTemplate, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *postgresRepository) GetRequirementTemplates(ctx context.Context) ([]*models.RequirementTemplate, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *postgresRepository) AddTickets(ctx context.Context, giveawayID string, userID int64, count int64) error {
	return fmt.Errorf("not implemented")
}

func (r *postgresRepository) GetUserTickets(ctx context.Context, giveawayID string, userID int64) (int, error) {
	return 0, fmt.Errorf("not implemented")
}

func (r *postgresRepository) GetTotalTickets(ctx context.Context, giveawayID string) (int, error) {
	return 0, fmt.Errorf("not implemented")
}

func (r *postgresRepository) GetAllTicketsTx(ctx context.Context, tx repository.Transaction, giveawayID string) (map[int64]int, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *postgresRepository) GetWinRecord(ctx context.Context, id string) (*models.WinRecord, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *postgresRepository) GetWinRecordsByGiveaway(ctx context.Context, giveawayID string) ([]*models.WinRecord, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *postgresRepository) UpdateWinRecord(ctx context.Context, record *models.WinRecord) error {
	return fmt.Errorf("not implemented")
}

func (r *postgresRepository) UpdateWinRecordTx(ctx context.Context, tx repository.Transaction, record *models.WinRecord) error {
	return fmt.Errorf("not implemented")
}

func (r *postgresRepository) DistributePrizeTx(ctx context.Context, tx repository.Transaction, giveawayID string, userID int64, prizeID string) error {
	postgresTx, ok := tx.(*postgresTransaction)
	if !ok {
		return fmt.Errorf("invalid transaction type")
	}

	query := `
		UPDATE win_records 
		SET status = 'distributed', received_at = NOW()
		WHERE giveaway_id = $1 AND user_id = $2 AND prize_id = $3
	`

	_, err := postgresTx.tx.ExecContext(ctx, query, giveawayID, userID, prizeID)
	if err != nil {
		return fmt.Errorf("failed to distribute prize: %w", err)
	}

	return nil
}

func (r *postgresRepository) GetCreator(ctx context.Context, userID int64) (*usermodels.User, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *postgresRepository) GetUser(ctx context.Context, userID int64) (*usermodels.User, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *postgresRepository) RemoveFromActive(ctx context.Context, id string) error {
	return fmt.Errorf("not implemented")
}

func (r *postgresRepository) DeleteParticipantsCount(ctx context.Context, id string) error {
	return fmt.Errorf("not implemented")
}

func (r *postgresRepository) DeletePrizes(ctx context.Context, id string) error {
	return fmt.Errorf("not implemented")
}

func (r *postgresRepository) GetByCreatorAndStatus(ctx context.Context, userID int64, statuses []models.GiveawayStatus) ([]*models.Giveaway, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *postgresRepository) GetByParticipantAndStatus(ctx context.Context, userID int64, statuses []models.GiveawayStatus) ([]*models.Giveaway, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *postgresRepository) AcquireLock(ctx context.Context, key string, timeout time.Duration) error {
	return fmt.Errorf("not implemented")
}

func (r *postgresRepository) ReleaseLock(ctx context.Context, key string) error {
	return fmt.Errorf("not implemented")
}

func (r *postgresRepository) CleanupInconsistentData(ctx context.Context) error {
	return fmt.Errorf("not implemented")
}

func (r *postgresRepository) AddToProcessingSet(ctx context.Context, id string) bool {
	return false
}

func (r *postgresRepository) RemoveFromProcessingSet(ctx context.Context, id string) error {
	return fmt.Errorf("not implemented")
}

func (r *postgresRepository) GetExpiredGiveaways(ctx context.Context, now int64) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *postgresRepository) GetTopGiveaways(ctx context.Context, limit int) ([]*models.Giveaway, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *postgresRepository) CancelGiveaway(ctx context.Context, giveawayID string) error {
	return fmt.Errorf("not implemented")
}

func (r *postgresRepository) GetCustomGiveaways(ctx context.Context) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *postgresRepository) SetChannelAvatar(ctx context.Context, channelID string, avatarURL string) error {
	return fmt.Errorf("not implemented")
}

func (r *postgresRepository) GetChannelAvatar(ctx context.Context, channelID string) (string, error) {
	return "", fmt.Errorf("not implemented")
}

func (r *postgresRepository) GetChannelUsername(ctx context.Context, channelID int64) (string, error) {
	return "", fmt.Errorf("not implemented")
}

func (r *postgresRepository) GetByCreator(ctx context.Context, creatorID int64) ([]*models.Giveaway, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *postgresRepository) GetActiveByCreator(ctx context.Context, creatorID int64) ([]*models.Giveaway, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *postgresRepository) GetHistoryByCreator(ctx context.Context, creatorID int64) ([]*models.Giveaway, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *postgresRepository) GetAwaitingActionByCreator(ctx context.Context, creatorID int64) ([]*models.Giveaway, error) {
	return nil, fmt.Errorf("not implemented")
}
