-- +goose Up
-- +goose StatementBegin
ALTER TABLE giveaway_sponsors ADD COLUMN IF NOT EXISTS channel_id BIGINT;
-- Optional: ensure (giveaway_id, channel_id) uniqueness if provided
CREATE UNIQUE INDEX IF NOT EXISTS giveaway_sponsors_unique_chan
  ON giveaway_sponsors (giveaway_id, channel_id)
  WHERE channel_id IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS giveaway_sponsors_unique_chan;
ALTER TABLE giveaway_sponsors DROP COLUMN IF EXISTS channel_id;
-- +goose StatementEnd
