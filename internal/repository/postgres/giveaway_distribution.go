package postgres

import (
	"context"
	"database/sql"
)

// distributePrizes handles the distribution of fixed and loose prizes to winners.
// It iterates through fixed prizes and assigns them to the corresponding place winner.
// Then it distributes loose prizes fairly across all winners.
func (r *GiveawayRepository) distributePrizes(ctx context.Context, tx *sql.Tx, id string, winners []int64, fixed map[int][]prize, loose []prize) error {
	winnersCount := len(winners)

	// Apply fixed prizes
	for place, list := range fixed {
		if place <= 0 || place > winnersCount {
			continue
		}
		uid := winners[place-1]
		for _, pr := range list {
			// Give one unit to fixed-place winner
			if _, err := tx.ExecContext(ctx, `INSERT INTO giveaway_winner_prizes (giveaway_id, user_id, prize_title, prize_description, quantity) VALUES ($1,$2,$3,$4,1)`, id, uid, pr.title, pr.desc); err != nil {
				return err
			}
			// Remaining quantity goes to loose distribution
			if pr.qty > 1 {
				loose = append(loose, prize{title: pr.title, desc: pr.desc, qty: pr.qty - 1})
			}
		}
	}

	// Distribute loose prizes
	idx := 0
	for _, pr := range loose {
		if winnersCount <= 0 {
			break
		}

		baseAmount := pr.qty / winnersCount
		remainder := pr.qty % winnersCount

		count := winnersCount
		if baseAmount == 0 {
			count = remainder
		}

		for i := 0; i < count; i++ {
			amount := baseAmount
			if i < remainder {
				amount++
			}
			if amount > 0 {
				uid := winners[(idx+i)%winnersCount]
				if _, err := tx.ExecContext(ctx, `INSERT INTO giveaway_winner_prizes (giveaway_id, user_id, prize_title, prize_description, quantity) VALUES ($1,$2,$3,$4,$5)`, id, uid, pr.title, pr.desc, amount); err != nil {
					return err
				}
			}
		}
		idx += pr.qty
	}
	return nil
}

// prize struct used internally for distribution
type prize struct {
	place       sql.NullInt64
	title, desc string
	qty         int
}

