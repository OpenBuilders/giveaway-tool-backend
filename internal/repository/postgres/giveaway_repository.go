package postgres

import (
	"context"
	"database/sql"
	"math/rand"

	dg "github.com/your-org/giveaway-backend/internal/domain/giveaway"
)

// GiveawayRepository persists giveaways and their nested entities.
type GiveawayRepository struct {
	db *sql.DB
}

func NewGiveawayRepository(db *sql.DB) *GiveawayRepository { return &GiveawayRepository{db: db} }

// Create inserts giveaway with prizes and sponsors in a single transaction.
func (r *GiveawayRepository) Create(ctx context.Context, g *dg.Giveaway) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	const qGiveaway = `
	INSERT INTO giveaways (id, creator_id, title, description, started_at, ends_at, duration, winners_count, status, created_at, updated_at)
	VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`
	_, err = tx.ExecContext(ctx, qGiveaway,
		g.ID, g.CreatorID, g.Title, g.Description, g.StartedAt, g.EndsAt, g.Duration, g.MaxWinnersCount, g.Status, g.CreatedAt, g.UpdatedAt,
	)
	if err != nil {
		return err
	}

	const qPrize = `INSERT INTO giveaway_prizes (giveaway_id, place, title, description, quantity) VALUES ($1,$2,$3,$4,COALESCE($5,1))`
	for _, p := range g.Prizes {
		var placeVal interface{}
		if p.Place != nil {
			placeVal = *p.Place
		} else {
			placeVal = nil
		}
		qty := p.Quantity
		if qty <= 0 {
			qty = 1
		}
		if _, err = tx.ExecContext(ctx, qPrize, g.ID, placeVal, p.Title, p.Description, qty); err != nil {
			return err
		}
	}

	const qSponsor = `INSERT INTO giveaway_sponsors (giveaway_id, username, url, title, channel_id) VALUES ($1,$2,$3,$4,$5)`
	for _, s := range g.Sponsors {
		if _, err = tx.ExecContext(ctx, qSponsor, g.ID, s.Username, s.URL, s.Title, s.ID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetByID returns a giveaway with nested prizes and sponsors.
func (r *GiveawayRepository) GetByID(ctx context.Context, id string) (*dg.Giveaway, error) {
	const q = `
        SELECT id, creator_id, title, description, started_at, ends_at, duration, winners_count, status, created_at, updated_at
        FROM giveaways WHERE id=$1`
	var g dg.Giveaway
	row := r.db.QueryRowContext(ctx, q, id)
	if err := row.Scan(&g.ID, &g.CreatorID, &g.Title, &g.Description, &g.StartedAt, &g.EndsAt, &g.Duration, &g.MaxWinnersCount, &g.Status, &g.CreatedAt, &g.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	// Prizes
	const qp = `SELECT place, title, description, quantity FROM giveaway_prizes WHERE giveaway_id=$1 ORDER BY place NULLS LAST, place ASC`
	rows, err := r.db.QueryContext(ctx, qp, id)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var (
				place sql.NullInt64
				p     dg.PrizePlace
			)
			if err := rows.Scan(&place, &p.Title, &p.Description, &p.Quantity); err != nil {
				return nil, err
			}
			if place.Valid {
				v := int(place.Int64)
				p.Place = &v
			}
			g.Prizes = append(g.Prizes, p)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}
	// Sponsors
	const qs = `SELECT username, url, title, channel_id FROM giveaway_sponsors WHERE giveaway_id=$1`
	srows, err := r.db.QueryContext(ctx, qs, id)
	if err == nil {
		defer srows.Close()
		for srows.Next() {
			var s dg.ChannelInfo
			if err := srows.Scan(&s.Username, &s.URL, &s.Title, &s.ID); err != nil {
				return nil, err
			}
			g.Sponsors = append(g.Sponsors, s)
		}
		if err := srows.Err(); err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}
	return &g, nil
}

// ListByCreator returns giveaways for a specific creator ordered by created_at desc.
func (r *GiveawayRepository) ListByCreator(ctx context.Context, creatorID int64, limit, offset int) ([]dg.Giveaway, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	const q = `
        SELECT id, creator_id, title, description, started_at, ends_at, duration, winners_count, status, created_at, updated_at
        FROM giveaways WHERE creator_id=$1
        ORDER BY created_at DESC
        LIMIT $2 OFFSET $3`
	rows, err := r.db.QueryContext(ctx, q, creatorID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []dg.Giveaway
	for rows.Next() {
		var g dg.Giveaway
		if err := rows.Scan(&g.ID, &g.CreatorID, &g.Title, &g.Description, &g.StartedAt, &g.EndsAt, &g.Duration, &g.MaxWinnersCount, &g.Status, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// UpdateStatus updates the giveaway status only.
func (r *GiveawayRepository) UpdateStatus(ctx context.Context, id string, status dg.GiveawayStatus) error {
	const q = `UPDATE giveaways SET status=$2, updated_at=now() WHERE id=$1`
	_, err := r.db.ExecContext(ctx, q, id, status)
	return err
}

// DeleteByOwner removes a giveaway only if the requester is the creator.
// Returns true if a row was deleted, false otherwise.
func (r *GiveawayRepository) DeleteByOwner(ctx context.Context, id string, ownerID int64) (bool, error) {
	const q = `DELETE FROM giveaways WHERE id=$1 AND creator_id=$2`
	res, err := r.db.ExecContext(ctx, q, id, ownerID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// Join adds a participant if not the creator; does nothing if creator.
func (r *GiveawayRepository) Join(ctx context.Context, id string, userID int64) error {
	const q = `
        INSERT INTO giveaway_participants (giveaway_id, user_id)
        SELECT $1, $2
        WHERE EXISTS (
            SELECT 1 FROM giveaways g
            WHERE g.id=$1 AND g.creator_id<>$2 AND g.status='active'
        )
        ON CONFLICT DO NOTHING`
	_, err := r.db.ExecContext(ctx, q, id, userID)
	return err
}

// FinishExpired marks finished giveaways whose ends_at passed and in scheduled/active.
func (r *GiveawayRepository) FinishExpired(ctx context.Context) (int64, error) {
	const q = `
        UPDATE giveaways
        SET status='finished', updated_at=now()
        WHERE ends_at <= now() AND status IN ('scheduled','active')`
	res, err := r.db.ExecContext(ctx, q)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ListExpiredIDs returns IDs of giveaways that should be finished now.
func (r *GiveawayRepository) ListExpiredIDs(ctx context.Context) ([]string, error) {
	const q = `SELECT id FROM giveaways WHERE ends_at <= now() AND status IN ('scheduled','active') ORDER BY ends_at ASC`
	rows, err := r.db.QueryContext(ctx, q)
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
	return ids, rows.Err()
}

// FinishOneWithDistribution finalizes a single giveaway: selects winners by place, assigns fixed-place prizes,
// and randomly distributes unassigned prizes among winners without a fixed prize. If extra prizes remain,
// distributes in round-robin starting from place 1.
func (r *GiveawayRepository) FinishOneWithDistribution(ctx context.Context, id string, winnersCount int) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Lock giveaway row to prevent concurrent finishing
	var status string
	if err = tx.QueryRowContext(ctx, `SELECT status FROM giveaways WHERE id=$1 FOR UPDATE`, id).Scan(&status); err != nil {
		return err
	}
	if status == "finished" {
		return tx.Commit()
	}

	// Collect participants (shuffle for randomness)
	rows, err := tx.QueryContext(ctx, `SELECT user_id FROM giveaway_participants WHERE giveaway_id=$1`, id)
	if err != nil {
		return err
	}
	var participants []int64
	for rows.Next() {
		var uid int64
		if err := rows.Scan(&uid); err != nil {
			rows.Close()
			return err
		}
		participants = append(participants, uid)
	}
	rows.Close()
	rand.Shuffle(len(participants), func(i, j int) { participants[i], participants[j] = participants[j], participants[i] })

	// Prepare winners slice size winnersCount or participants length
	if winnersCount > len(participants) {
		winnersCount = len(participants)
	}
	winners := make([]int64, 0, winnersCount)
	for i := 0; i < winnersCount; i++ {
		winners = append(winners, participants[i])
	}

	// Assign winners to places 1..winnersCount
	for place := 1; place <= winnersCount; place++ {
		if _, err = tx.ExecContext(ctx, `INSERT INTO giveaway_winners (giveaway_id, place, user_id) VALUES ($1,$2,$3)`, id, place, winners[place-1]); err != nil {
			return err
		}
	}

	// Load prizes
	pRows, err := tx.QueryContext(ctx, `SELECT place, title, description, quantity FROM giveaway_prizes WHERE giveaway_id=$1`, id)
	if err != nil {
		return err
	}
	type prize struct {
		place       sql.NullInt64
		title, desc string
		qty         int
	}
	var fixed = map[int][]prize{}
	var loose []prize
	for pRows.Next() {
		var pr prize
		if err := pRows.Scan(&pr.place, &pr.title, &pr.desc, &pr.qty); err != nil {
			pRows.Close()
			return err
		}
		if pr.qty <= 0 {
			pr.qty = 1
		}
		if pr.place.Valid {
			fixed[int(pr.place.Int64)] = append(fixed[int(pr.place.Int64)], pr)
		} else {
			loose = append(loose, pr)
		}
	}
	pRows.Close()

	// Apply fixed-place prizes
	for place, list := range fixed {
		if place <= 0 || place > winnersCount {
			continue
		}
		uid := winners[place-1]
		for _, pr := range list {
			if _, err = tx.ExecContext(ctx, `INSERT INTO giveaway_winner_prizes (giveaway_id, user_id, prize_title, prize_description) VALUES ($1,$2,$3,$4)`, id, uid, pr.title, pr.desc); err != nil {
				return err
			}
		}
	}

	// Build list of winners without fixed prize
	without := make([]int64, 0, winnersCount)
	for place := 1; place <= winnersCount; place++ {
		if len(fixed[place]) == 0 {
			without = append(without, winners[place-1])
		}
	}
	if len(without) > 1 {
		rand.Shuffle(len(without), func(i, j int) { without[i], without[j] = without[j], without[i] })
	}

	// Distribute loose prizes: first pass give one per winner without fixed
	idx := 0
	for _, pr := range loose {
		remaining := pr.qty
		// First pass
		for i := 0; i < len(without) && remaining > 0; i++ {
			if _, err = tx.ExecContext(ctx, `INSERT INTO giveaway_winner_prizes (giveaway_id, user_id, prize_title, prize_description) VALUES ($1,$2,$3,$4)`, id, without[i], pr.title, pr.desc); err != nil {
				return err
			}
			remaining--
		}
		// Round-robin second pass starting from place 1
		for remaining > 0 && winnersCount > 0 {
			uid := winners[idx%winnersCount]
			if _, err = tx.ExecContext(ctx, `INSERT INTO giveaway_winner_prizes (giveaway_id, user_id, prize_title, prize_description) VALUES ($1,$2,$3,$4)`, id, uid, pr.title, pr.desc); err != nil {
				return err
			}
			remaining--
			idx++
		}
	}

	// Mark giveaway finished
	if _, err = tx.ExecContext(ctx, `UPDATE giveaways SET status='finished', updated_at=now() WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ListFinishedByCreator returns finished giveaways for the creator.
func (r *GiveawayRepository) ListFinishedByCreator(ctx context.Context, creatorID int64, limit, offset int) ([]dg.Giveaway, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	const q = `
        SELECT id, creator_id, title, description, started_at, ends_at, duration, winners_count, status, created_at, updated_at
        FROM giveaways
        WHERE creator_id=$1 AND status='finished'
        ORDER BY ends_at DESC
        LIMIT $2 OFFSET $3`
	rows, err := r.db.QueryContext(ctx, q, creatorID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []dg.Giveaway
	for rows.Next() {
		var g dg.Giveaway
		if err := rows.Scan(&g.ID, &g.CreatorID, &g.Title, &g.Description, &g.StartedAt, &g.EndsAt, &g.Duration, &g.MaxWinnersCount, &g.Status, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
