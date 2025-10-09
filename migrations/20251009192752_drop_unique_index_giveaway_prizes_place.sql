-- +goose Up
-- +goose StatementBegin
DROP INDEX IF EXISTS giveaway_prizes_unique_place;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Recreate unique index (note: will fail if duplicates exist)
CREATE UNIQUE INDEX IF NOT EXISTS giveaway_prizes_unique_place
  ON giveaway_prizes (giveaway_id, place)
  WHERE place IS NOT NULL;
-- +goose StatementEnd
