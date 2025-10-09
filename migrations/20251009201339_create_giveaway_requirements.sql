-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS giveaway_requirements (
    id BIGSERIAL PRIMARY KEY,
    giveaway_id TEXT NOT NULL REFERENCES giveaways(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('subscription')),
    channel_id BIGINT NULL,
    channel_username TEXT NULL
);
CREATE INDEX IF NOT EXISTS giveaway_requirements_giveaway_idx ON giveaway_requirements (giveaway_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS giveaway_requirements;
-- +goose StatementEnd
