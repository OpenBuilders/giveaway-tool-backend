-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS giveaway_participants (
    giveaway_id TEXT NOT NULL REFERENCES giveaways(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    joined_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (giveaway_id, user_id)
);

CREATE INDEX IF NOT EXISTS giveaway_participants_giveaway_idx ON giveaway_participants (giveaway_id);
CREATE INDEX IF NOT EXISTS giveaway_participants_user_idx ON giveaway_participants (user_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS giveaway_participants;
-- +goose StatementEnd
