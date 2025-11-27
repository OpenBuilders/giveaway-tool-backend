package postgres

import (
	"context"
	"database/sql"

	dg "github.com/open-builders/giveaway-backend/internal/domain/giveaway"
	"github.com/open-builders/giveaway-backend/internal/utils/random"
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

	const qSponsor = `INSERT INTO giveaway_sponsors (giveaway_id, username, url, title, channel_id, avatar_url) VALUES ($1,$2,$3,$4,$5,$6)`
	for _, s := range g.Sponsors {
		var uname interface{}
		if s.Username != "" {
			uname = s.Username
		} else {
			uname = nil
		}
		if _, err = tx.ExecContext(ctx, qSponsor, g.ID, uname, s.URL, s.Title, s.ID, s.AvatarURL); err != nil {
			return err
		}
	}

	// Requirements
	if len(g.Requirements) > 0 {
		const qReq = `INSERT INTO giveaway_requirements (giveaway_id, type, channel_id, channel_username, name, description, ton_min_balance_nano, jetton_address, jetton_min_amount)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`
		for _, rqm := range g.Requirements {
			var cid interface{}

			if rqm.ChannelID != 0 {
				cid = rqm.ChannelID
			} else {
				cid = nil
			}
			var tonMin interface{}
			if rqm.TonMinBalanceNano != 0 {
				tonMin = rqm.TonMinBalanceNano
			} else {
				tonMin = nil
			}
			var jetMin interface{}
			if rqm.JettonMinAmount != 0 {
				jetMin = rqm.JettonMinAmount
			} else {
				jetMin = nil
			}
			if _, err = tx.ExecContext(ctx, qReq, g.ID, string(rqm.Type), cid, rqm.ChannelUsername, rqm.ChannelTitle, rqm.Description, tonMin, rqm.JettonAddress, jetMin); err != nil {
				return err
			}
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
	// Participants count
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM giveaway_participants WHERE giveaway_id=$1`, id).Scan(&g.ParticipantsCount); err != nil {
		return nil, err
	}

	// Sponsors
	const qs = `SELECT COALESCE(username,'') AS username, url, title, channel_id, COALESCE(avatar_url,'') AS avatar_url FROM giveaway_sponsors WHERE giveaway_id=$1`
	srows, err := r.db.QueryContext(ctx, qs, id)
	if err == nil {
		defer srows.Close()
		for srows.Next() {
			var s dg.ChannelInfo
			if err := srows.Scan(&s.Username, &s.URL, &s.Title, &s.ID, &s.AvatarURL); err != nil {
				return nil, err
			}
			// Fallback: if URL not stored, build from username
			if s.URL == "" && s.Username != "" {
				s.URL = "https://t.me/" + s.Username
			}
			g.Sponsors = append(g.Sponsors, s)
		}
		if err := srows.Err(); err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}
	// If finished or completed, load winners and their prizes
	if g.Status == dg.GiveawayStatusFinished || g.Status == dg.GiveawayStatusCompleted {
		// Winners by place
		wrows, err := r.db.QueryContext(ctx, `SELECT place, user_id FROM giveaway_winners WHERE giveaway_id=$1 ORDER BY place ASC`, id)
		if err != nil {
			return nil, err
		}
		type winner struct {
			place int
			user  int64
		}
		var winners []winner
		for wrows.Next() {
			var pl int
			var uid int64
			if err := wrows.Scan(&pl, &uid); err != nil {
				wrows.Close()
				return nil, err
			}
			winners = append(winners, winner{place: pl, user: uid})
		}
		wrows.Close()
		// Prizes per user
		prizemap := map[int64][]dg.WinnerPrize{}
		prows, err := r.db.QueryContext(ctx, `SELECT user_id, prize_title, prize_description FROM giveaway_winner_prizes WHERE giveaway_id=$1`, id)
		if err != nil {
			return nil, err
		}
		for prows.Next() {
			var uid int64
			var t, d string
			if err := prows.Scan(&uid, &t, &d); err != nil {
				prows.Close()
				return nil, err
			}
			prizemap[uid] = append(prizemap[uid], dg.WinnerPrize{Title: t, Description: d})
		}
		prows.Close()
		// Build DTO
		for _, w := range winners {
			g.Winners = append(g.Winners, dg.Winner{Place: w.place, UserID: w.user, Prizes: prizemap[w.user]})
		}
	}

	// Load requirements (support older schema without name/description)
	rqrows, err := r.db.QueryContext(ctx, `SELECT type, channel_id, channel_username, name, description, ton_min_balance_nano, jetton_address, jetton_min_amount FROM giveaway_requirements WHERE giveaway_id=$1`, id)
	if err == nil {
		defer rqrows.Close()
		for rqrows.Next() {
			var t string
			var cid sql.NullInt64
			var uname sql.NullString
			var name sql.NullString
			var desc sql.NullString
			var ton sql.NullInt64
			var jaddr sql.NullString
			var jmin sql.NullInt64
			if err := rqrows.Scan(&t, &cid, &uname, &name, &desc, &ton, &jaddr, &jmin); err != nil {
				return nil, err
			}
			req := dg.Requirement{Type: dg.RequirementType(t)}
			if cid.Valid {
				req.ChannelID = cid.Int64
			}
			if uname.Valid {
				req.ChannelUsername = uname.String
			}
			if name.Valid {
				req.ChannelTitle = name.String
			}
			if desc.Valid {
				req.Description = desc.String
			}
			if ton.Valid {
				req.TonMinBalanceNano = ton.Int64
			}
			if jaddr.Valid {
				req.JettonAddress = jaddr.String
			}
			if jmin.Valid {
				req.JettonMinAmount = jmin.Int64
			}
			g.Requirements = append(g.Requirements, req)
		}
	} else {
		// Fallback for old schema (no name/description columns)
		rqrows2, err2 := r.db.QueryContext(ctx, `SELECT type, channel_id, channel_username FROM giveaway_requirements WHERE giveaway_id=$1`, id)
		if err2 == nil {
			defer rqrows2.Close()
			for rqrows2.Next() {
				var t string
				var cid sql.NullInt64
				var uname sql.NullString
				if err := rqrows2.Scan(&t, &cid, &uname); err != nil {
					return nil, err
				}
				req := dg.Requirement{Type: dg.RequirementType(t)}
				if cid.Valid {
					req.ChannelID = cid.Int64
				}
				if uname.Valid {
					req.ChannelUsername = uname.String
				}
				g.Requirements = append(g.Requirements, req)
			}
		}
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
	out := make([]dg.Giveaway, 0)
	for rows.Next() {
		var g dg.Giveaway
		if err := rows.Scan(&g.ID, &g.CreatorID, &g.Title, &g.Description, &g.StartedAt, &g.EndsAt, &g.Duration, &g.MaxWinnersCount, &g.Status, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		// Load sponsors for each giveaway (same as in GetByID)
		const qs = `SELECT COALESCE(username,'') AS username, url, title, channel_id, COALESCE(avatar_url,'') AS avatar_url FROM giveaway_sponsors WHERE giveaway_id=$1`
		srows, err := r.db.QueryContext(ctx, qs, g.ID)
		if err == nil {
			for srows.Next() {
				var s dg.ChannelInfo
				if err := srows.Scan(&s.Username, &s.URL, &s.Title, &s.ID, &s.AvatarURL); err != nil {
					srows.Close()
					return nil, err
				}
				if s.URL == "" && s.Username != "" {
					s.URL = "https://t.me/" + s.Username
				}
				g.Sponsors = append(g.Sponsors, s)
			}
			srows.Close()
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
        SET status='completed', updated_at=now()
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

	// Lock giveaway row to prevent concurrent finishing and prefetch requirements presence
	var status string
	if err = tx.QueryRowContext(ctx, `SELECT status FROM giveaways WHERE id=$1 FOR UPDATE`, id).Scan(&status); err != nil {
		return err
	}
	if status == "finished" {
		return tx.Commit()
	}

	// If custom requirement exists -> set pending and exit
	var hasCustom bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM giveaway_requirements WHERE giveaway_id=$1 AND type='custom')`, id).Scan(&hasCustom); err != nil {
		return err
	}
	if hasCustom {
		if _, err = tx.ExecContext(ctx, `UPDATE giveaways SET status='pending', updated_at=now() WHERE id=$1`, id); err != nil {
			return err
		}
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
	if err := random.Shuffle(participants); err != nil {
		return err
	}

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
			// Always give one unit to the fixed-place winner
			if _, err = tx.ExecContext(ctx, `INSERT INTO giveaway_winner_prizes (giveaway_id, user_id, prize_title, prize_description) VALUES ($1,$2,$3,$4)`, id, uid, pr.title, pr.desc); err != nil {
				return err
			}
			// Distribute remaining quantity across all winners in the general loose pass
			if pr.qty > 1 {
				loose = append(loose, prize{title: pr.title, desc: pr.desc, qty: pr.qty - 1})
			}
		}
	}

	// Build list of winners without fixed prize
	// Deprecated: previously used to prioritize winners without fixed prizes.
	// New distribution fills the full circle of winners first, then continues round-robin.

	// Distribute loose prizes: first pass give one per winner without fixed
	idx := 0
	for _, pr := range loose {
		remaining := pr.qty
		// First pass: fill the entire circle of winners (one per winner) before any second unit
		if winnersCount > 0 {
			firstRound := remaining
			if firstRound > winnersCount {
				firstRound = winnersCount
			}
			for i := 0; i < firstRound; i++ {
				uid := winners[(idx+i)%winnersCount]
				if _, err = tx.ExecContext(ctx, `INSERT INTO giveaway_winner_prizes (giveaway_id, user_id, prize_title, prize_description) VALUES ($1,$2,$3,$4)`, id, uid, pr.title, pr.desc); err != nil {
					return err
				}
			}
			idx += firstRound
			remaining -= firstRound
		}
		// Round-robin for remaining units
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
	if _, err = tx.ExecContext(ctx, `UPDATE giveaways SET status='completed', updated_at=now() WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// IsParticipant returns true if the user participated in the giveaway.
func (r *GiveawayRepository) IsParticipant(ctx context.Context, id string, userID int64) (bool, error) {
	const q = `SELECT 1 FROM giveaway_participants WHERE giveaway_id=$1 AND user_id=$2 LIMIT 1`
	var one int
	err := r.db.QueryRowContext(ctx, q, id, userID).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// IsWinner returns true if the user is among winners of the giveaway.
func (r *GiveawayRepository) IsWinner(ctx context.Context, id string, userID int64) (bool, error) {
	const q = `SELECT 1 FROM giveaway_winners WHERE giveaway_id=$1 AND user_id=$2 LIMIT 1`
	var one int
	err := r.db.QueryRowContext(ctx, q, id, userID).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// FinishWithWinners finalizes a giveaway using the provided winners list (ordered by place).
// It assigns fixed and loose prizes similarly to FinishOneWithDistribution.
func (r *GiveawayRepository) FinishWithWinners(ctx context.Context, id string, winners []int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Lock and check status
	var status string
	if err = tx.QueryRowContext(ctx, `SELECT status FROM giveaways WHERE id=$1 FOR UPDATE`, id).Scan(&status); err != nil {
		return err
	}
	if status == "finished" {
		return tx.Commit()
	}

	winnersCount := len(winners)
	if winnersCount == 0 {
		// no winners, set status to completed
		if _, err = tx.ExecContext(ctx, `UPDATE giveaways SET status='completed', updated_at=now() WHERE id=$1`, id); err != nil {
			return err
		}

		return tx.Commit()
	}

	// Persist winners per place
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

	// Apply fixed prizes
	for place, list := range fixed {
		if place <= 0 || place > winnersCount {
			continue
		}
		uid := winners[place-1]
		for _, pr := range list {
			// Give one to the fixed-place winner
			if _, err = tx.ExecContext(ctx, `INSERT INTO giveaway_winner_prizes (giveaway_id, user_id, prize_title, prize_description) VALUES ($1,$2,$3,$4)`, id, uid, pr.title, pr.desc); err != nil {
				return err
			}
			// Remaining quantity goes to loose distribution
			if pr.qty > 1 {
				loose = append(loose, prize{title: pr.title, desc: pr.desc, qty: pr.qty - 1})
			}
		}
	}

	// New strategy: fill the full circle of winners first, then continue round-robin

	// Distribute loose prizes
	idx := 0
	for _, pr := range loose {
		remaining := pr.qty
		// First pass: one unit per winner until we cover all winners or run out
		if winnersCount > 0 {
			firstRound := remaining
			if firstRound > winnersCount {
				firstRound = winnersCount
			}
			for i := 0; i < firstRound; i++ {
				uid := winners[(idx+i)%winnersCount]
				if _, err = tx.ExecContext(ctx, `INSERT INTO giveaway_winner_prizes (giveaway_id, user_id, prize_title, prize_description) VALUES ($1,$2,$3,$4)`, id, uid, pr.title, pr.desc); err != nil {
					return err
				}
			}
			idx += firstRound
			remaining -= firstRound
		}
		for remaining > 0 && winnersCount > 0 {
			uid := winners[idx%winnersCount]
			if _, err = tx.ExecContext(ctx, `INSERT INTO giveaway_winner_prizes (giveaway_id, user_id, prize_title, prize_description) VALUES ($1,$2,$3,$4)`, id, uid, pr.title, pr.desc); err != nil {
				return err
			}
			remaining--
			idx++
		}
	}

	if _, err = tx.ExecContext(ctx, `UPDATE giveaways SET status='completed', updated_at=now() WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// SetManualWinners replaces winners and distributes prizes while keeping giveaway in pending status.
// Existing winners and winner_prizes are deleted and replaced.
func (r *GiveawayRepository) SetManualWinners(ctx context.Context, id string, winners []int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Ensure current status is pending and lock row
	var status string
	if err = tx.QueryRowContext(ctx, `SELECT status FROM giveaways WHERE id=$1 FOR UPDATE`, id).Scan(&status); err != nil {
		return err
	}
	if status != "pending" {
		return tx.Commit()
	}

	// Clear previous winners and prizes
	if _, err = tx.ExecContext(ctx, `DELETE FROM giveaway_winner_prizes WHERE giveaway_id=$1`, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM giveaway_winners WHERE giveaway_id=$1`, id); err != nil {
		return err
	}

	if len(winners) == 0 {
		return tx.Commit()
	}

	// Persist winners per place
	for place := 1; place <= len(winners); place++ {
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

	winnersCount := len(winners)
	// Apply fixed prizes
	for place, list := range fixed {
		if place <= 0 || place > winnersCount {
			continue
		}
		uid := winners[place-1]
		for _, pr := range list {
			// Give one unit to fixed-place winner; distribute the rest as loose
			if _, err = tx.ExecContext(ctx, `INSERT INTO giveaway_winner_prizes (giveaway_id, user_id, prize_title, prize_description) VALUES ($1,$2,$3,$4)`, id, uid, pr.title, pr.desc); err != nil {
				return err
			}
			if pr.qty > 1 {
				loose = append(loose, prize{title: pr.title, desc: pr.desc, qty: pr.qty - 1})
			}
		}
	}

	// Distribute loose prizes using first pass one-per across all winners and then round-robin
	idx := 0
	for _, pr := range loose {
		remaining := pr.qty
		// First pass: cover all winners once if possible
		if winnersCount > 0 {
			firstRound := remaining
			if firstRound > winnersCount {
				firstRound = winnersCount
			}
			for i := 0; i < firstRound; i++ {
				uid := winners[(idx+i)%winnersCount]
				if _, err = tx.ExecContext(ctx, `INSERT INTO giveaway_winner_prizes (giveaway_id, user_id, prize_title, prize_description) VALUES ($1,$2,$3,$4)`, id, uid, pr.title, pr.desc); err != nil {
					return err
				}
			}
			idx += firstRound
			remaining -= firstRound
		}
		// Second pass: round-robin across all winners
		for remaining > 0 && winnersCount > 0 {
			uid := winners[idx%winnersCount]
			if _, err = tx.ExecContext(ctx, `INSERT INTO giveaway_winner_prizes (giveaway_id, user_id, prize_title, prize_description) VALUES ($1,$2,$3,$4)`, id, uid, pr.title, pr.desc); err != nil {
				return err
			}
			remaining--
			idx++
		}
	}

	// Keep status pending
	return tx.Commit()
}

// ListWinnersWithPrizes returns winners ordered by place with their prizes regardless of giveaway status.
func (r *GiveawayRepository) ListWinnersWithPrizes(ctx context.Context, id string) ([]dg.Winner, error) {
	// Winners by place
	wrows, err := r.db.QueryContext(ctx, `SELECT place, user_id FROM giveaway_winners WHERE giveaway_id=$1 ORDER BY place ASC`, id)
	if err != nil {
		return nil, err
	}
	type winner struct {
		place int
		user  int64
	}
	var winners []winner
	for wrows.Next() {
		var pl int
		var uid int64
		if err := wrows.Scan(&pl, &uid); err != nil {
			wrows.Close()
			return nil, err
		}
		winners = append(winners, winner{place: pl, user: uid})
	}
	wrows.Close()

	prizemap := map[int64][]dg.WinnerPrize{}
	prows, err := r.db.QueryContext(ctx, `SELECT user_id, prize_title, prize_description FROM giveaway_winner_prizes WHERE giveaway_id=$1`, id)
	if err != nil {
		return nil, err
	}
	for prows.Next() {
		var uid int64
		var t, d string
		if err := prows.Scan(&uid, &t, &d); err != nil {
			prows.Close()
			return nil, err
		}
		prizemap[uid] = append(prizemap[uid], dg.WinnerPrize{Title: t, Description: d})
	}
	prows.Close()

	out := make([]dg.Winner, 0, len(winners))
	for _, w := range winners {
		out = append(out, dg.Winner{Place: w.place, UserID: w.user, Prizes: prizemap[w.user]})
	}
	return out, nil
}

// ClearWinners removes all winners and their prizes for the giveaway.
func (r *GiveawayRepository) ClearWinners(ctx context.Context, id string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Lock row to serialize concurrent operations
	var one string
	if err = tx.QueryRowContext(ctx, `SELECT id FROM giveaways WHERE id=$1 FOR UPDATE`, id).Scan(&one); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM giveaway_winner_prizes WHERE giveaway_id=$1`, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM giveaway_winners WHERE giveaway_id=$1`, id); err != nil {
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
        WHERE creator_id=$1 AND status='completed'
        ORDER BY ends_at DESC
        LIMIT $2 OFFSET $3`
	rows, err := r.db.QueryContext(ctx, q, creatorID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]dg.Giveaway, 0)
	for rows.Next() {
		var g dg.Giveaway
		if err := rows.Scan(&g.ID, &g.CreatorID, &g.Title, &g.Description, &g.StartedAt, &g.EndsAt, &g.Duration, &g.MaxWinnersCount, &g.Status, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// ListActive returns active giveaways with participants count, filtered by minParticipants and paginated.
func (r *GiveawayRepository) ListActive(ctx context.Context, limit, offset, minParticipants int) ([]dg.Giveaway, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	if minParticipants < 0 {
		minParticipants = 0
	}
	const q = `
        SELECT g.id, g.creator_id, g.title, g.description, g.started_at, g.ends_at,
               g.duration, g.winners_count, g.status, g.created_at, g.updated_at,
               COALESCE(pc.cnt,0) as participants_count
        FROM giveaways g
        LEFT JOIN (
            SELECT giveaway_id, COUNT(*)::int AS cnt
            FROM giveaway_participants
            GROUP BY giveaway_id
        ) pc ON pc.giveaway_id = g.id
        WHERE g.status='active' AND COALESCE(pc.cnt,0) >= $3
        ORDER BY pc.cnt DESC NULLS LAST, g.created_at DESC
        LIMIT $1 OFFSET $2`
	rows, err := r.db.QueryContext(ctx, q, limit, offset, minParticipants)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]dg.Giveaway, 0)
	for rows.Next() {
		var g dg.Giveaway
		if err := rows.Scan(&g.ID, &g.CreatorID, &g.Title, &g.Description, &g.StartedAt, &g.EndsAt,
			&g.Duration, &g.MaxWinnersCount, &g.Status, &g.CreatedAt, &g.UpdatedAt, &g.ParticipantsCount); err != nil {
			return nil, err
		}
		// Load sponsors
		const qs = `SELECT COALESCE(username,'') AS username, url, title, channel_id, COALESCE(avatar_url,'') AS avatar_url FROM giveaway_sponsors WHERE giveaway_id=$1`
		srows, err := r.db.QueryContext(ctx, qs, g.ID)
		if err == nil {
			for srows.Next() {
				var s dg.ChannelInfo
				if err := srows.Scan(&s.Username, &s.URL, &s.Title, &s.ID, &s.AvatarURL); err != nil {
					srows.Close()
					return nil, err
				}
				if s.URL == "" && s.Username != "" {
					s.URL = "https://t.me/" + s.Username
				}
				g.Sponsors = append(g.Sponsors, s)
			}
			srows.Close()
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// GetParticipants returns all participant user IDs for a giveaway.
func (r *GiveawayRepository) GetParticipants(ctx context.Context, id string) ([]int64, error) {
	const q = `SELECT user_id FROM giveaway_participants WHERE giveaway_id=$1`
	rows, err := r.db.QueryContext(ctx, q, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var participants []int64
	for rows.Next() {
		var uid int64
		if err := rows.Scan(&uid); err != nil {
			return nil, err
		}
		participants = append(participants, uid)
	}
	return participants, rows.Err()
}

// RemoveRequirementsByChannelID removes any requirements that depend on the given channel ID.
// Only deletes requirements for giveaways that are not yet finished (active, scheduled, pending).
func (r *GiveawayRepository) RemoveRequirementsByChannelID(ctx context.Context, channelID int64) error {
	const q = `
		DELETE FROM giveaway_requirements gr
		USING giveaways g
		WHERE gr.giveaway_id = g.id
		  AND gr.channel_id = $1
		  AND gr.type IN ('subscription', 'boost')
		  AND g.status IN ('active')`
	_, err := r.db.ExecContext(ctx, q, channelID)
	return err
}
