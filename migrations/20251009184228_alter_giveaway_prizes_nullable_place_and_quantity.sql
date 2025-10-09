-- +goose Up
-- +goose StatementBegin
-- Make place nullable and add quantity; keep PK to allow multiple unassigned rows
ALTER TABLE giveaway_prizes
    ALTER COLUMN place DROP NOT NULL,
    ADD COLUMN IF NOT EXISTS quantity INT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS id BIGSERIAL;

-- Switch to surrogate primary key
ALTER TABLE giveaway_prizes DROP CONSTRAINT IF EXISTS giveaway_prizes_pkey;
ALTER TABLE giveaway_prizes ADD PRIMARY KEY (id);

-- Ensure only one prize per fixed place per giveaway
CREATE UNIQUE INDEX IF NOT EXISTS giveaway_prizes_unique_place
  ON giveaway_prizes (giveaway_id, place)
  WHERE place IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS giveaway_prizes_unique_place;
ALTER TABLE giveaway_prizes DROP CONSTRAINT IF EXISTS giveaway_prizes_pkey;
-- Recreate previous composite PK (may fail if null places exist; for down migration simplicity we skip strict restoration)
ALTER TABLE giveaway_prizes ADD PRIMARY KEY (giveaway_id, place);
ALTER TABLE giveaway_prizes
    ALTER COLUMN place SET NOT NULL,
    DROP COLUMN IF EXISTS quantity,
    DROP COLUMN IF EXISTS id;
-- +goose StatementEnd
