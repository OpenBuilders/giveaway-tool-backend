-- +goose Up
-- +goose StatementBegin
-- Enum for giveaway status
CREATE TYPE giveaway_status AS ENUM ('scheduled', 'active', 'finished', 'cancelled');

-- Main giveaways table
CREATE TABLE IF NOT EXISTS giveaways (
    id TEXT PRIMARY KEY,
    creator_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    duration BIGINT NOT NULL CHECK (duration >= 0),
    winners_count INT NOT NULL CHECK (winners_count > 0),
    status giveaway_status NOT NULL DEFAULT 'scheduled',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Updated_at trigger
DROP TRIGGER IF EXISTS giveaways_set_updated_at ON giveaways;
CREATE TRIGGER giveaways_set_updated_at
  BEFORE UPDATE ON giveaways
  FOR EACH ROW
  EXECUTE FUNCTION set_updated_at();

-- Prizes per place for a giveaway
CREATE TABLE IF NOT EXISTS giveaway_prizes (
    giveaway_id TEXT NOT NULL REFERENCES giveaways(id) ON DELETE CASCADE,
    place INT NOT NULL CHECK (place > 0),
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (giveaway_id, place)
);

-- Sponsors list
CREATE TABLE IF NOT EXISTS giveaway_sponsors (
    giveaway_id TEXT NOT NULL REFERENCES giveaways(id) ON DELETE CASCADE,
    username TEXT NOT NULL,
    url TEXT NULL,
    title TEXT NULL,
    PRIMARY KEY (giveaway_id, username)
);

CREATE INDEX IF NOT EXISTS giveaways_creator_idx ON giveaways (creator_id);
CREATE INDEX IF NOT EXISTS giveaways_status_idx ON giveaways (status);
CREATE INDEX IF NOT EXISTS giveaways_ends_at_idx ON giveaways (ends_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS giveaway_sponsors;
DROP TABLE IF EXISTS giveaway_prizes;
DROP TRIGGER IF EXISTS giveaways_set_updated_at ON giveaways;
DROP TABLE IF EXISTS giveaways;
DROP TYPE IF EXISTS giveaway_status;
-- +goose StatementEnd
