-- +goose Up
-- +goose StatementBegin
ALTER TABLE giveaway_requirements
    ADD COLUMN IF NOT EXISTS name TEXT NULL,
    ADD COLUMN IF NOT EXISTS description TEXT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE giveaway_requirements
    DROP COLUMN IF EXISTS name,
    DROP COLUMN IF EXISTS description;
-- +goose StatementEnd


