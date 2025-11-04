-- +goose Up
-- +goose StatementBegin
-- Switch giveaway_sponsors to surrogate PK and partial unique indexes
ALTER TABLE giveaway_sponsors DROP CONSTRAINT IF EXISTS giveaway_sponsors_pkey;
ALTER TABLE giveaway_sponsors ADD COLUMN IF NOT EXISTS id BIGSERIAL;
-- Username may be missing -> allow NULL
ALTER TABLE giveaway_sponsors ALTER COLUMN username DROP NOT NULL;
-- Normalize empty usernames to NULL
UPDATE giveaway_sponsors SET username = NULL WHERE username = '';
-- Set new primary key on surrogate id
ALTER TABLE giveaway_sponsors ADD CONSTRAINT giveaway_sponsors_pkey PRIMARY KEY (id);
-- Ensure uniqueness by (giveaway_id, username) when username is present and non-empty
CREATE UNIQUE INDEX IF NOT EXISTS giveaway_sponsors_unique_username
  ON giveaway_sponsors (giveaway_id, username)
  WHERE username IS NOT NULL AND username <> '';
-- Note: unique index on (giveaway_id, channel_id) already exists from a prior migration
-- (migrations/20251009183832_add_channel_id_to_giveaway_sponsors.sql)
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Drop partial unique for username
DROP INDEX IF EXISTS giveaway_sponsors_unique_username;
-- Restore non-null usernames by converting NULL back to empty string
UPDATE giveaway_sponsors SET username = '' WHERE username IS NULL;
-- Drop surrogate PK and column
ALTER TABLE giveaway_sponsors DROP CONSTRAINT IF EXISTS giveaway_sponsors_pkey;
ALTER TABLE giveaway_sponsors DROP COLUMN IF EXISTS id;
-- Restore legacy composite primary key
ALTER TABLE giveaway_sponsors ADD CONSTRAINT giveaway_sponsors_pkey PRIMARY KEY (giveaway_id, username);
-- +goose StatementEnd


