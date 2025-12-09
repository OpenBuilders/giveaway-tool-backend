-- +goose Up
ALTER TABLE giveaway_winner_prizes ADD COLUMN quantity INTEGER NOT NULL DEFAULT 1;

-- +goose Down
ALTER TABLE giveaway_winner_prizes DROP COLUMN quantity;

