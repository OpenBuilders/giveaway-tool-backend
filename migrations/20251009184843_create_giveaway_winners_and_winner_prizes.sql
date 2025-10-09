-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS giveaway_winners (
    giveaway_id TEXT NOT NULL REFERENCES giveaways(id) ON DELETE CASCADE,
    place INT NOT NULL CHECK (place > 0),
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (giveaway_id, place)
);

CREATE TABLE IF NOT EXISTS giveaway_winner_prizes (
    id BIGSERIAL PRIMARY KEY,
    giveaway_id TEXT NOT NULL REFERENCES giveaways(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    prize_title TEXT NOT NULL,
    prize_description TEXT NOT NULL DEFAULT ''
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS giveaway_winner_prizes;
DROP TABLE IF EXISTS giveaway_winners;
-- +goose StatementEnd
