-- +goose Up
-- +goose StatementBegin
ALTER TABLE giveaway_sponsors ADD COLUMN IF NOT EXISTS avatar_url TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE giveaway_sponsors DROP COLUMN IF EXISTS avatar_url;
-- +goose StatementEnd


