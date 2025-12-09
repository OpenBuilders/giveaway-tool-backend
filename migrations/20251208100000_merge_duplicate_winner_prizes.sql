-- +goose Up
-- +goose StatementBegin
WITH to_update AS (
    SELECT
        min(id) as id,
        SUM(quantity) as new_quantity
    FROM giveaway_winner_prizes
    GROUP BY giveaway_id, user_id, prize_title, prize_description
    HAVING COUNT(*) > 1
)
UPDATE giveaway_winner_prizes
SET quantity = to_update.new_quantity
FROM to_update
WHERE giveaway_winner_prizes.id = to_update.id;

DELETE FROM giveaway_winner_prizes
WHERE id NOT IN (
    SELECT min(id)
    FROM giveaway_winner_prizes
    GROUP BY giveaway_id, user_id, prize_title, prize_description
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
INSERT INTO giveaway_winner_prizes (giveaway_id, user_id, prize_title, prize_description, quantity)
SELECT giveaway_id, user_id, prize_title, prize_description, 1
FROM giveaway_winner_prizes
CROSS JOIN generate_series(1, quantity - 1)
WHERE quantity > 1;

UPDATE giveaway_winner_prizes
SET quantity = 1
WHERE quantity > 1;
-- +goose StatementEnd

