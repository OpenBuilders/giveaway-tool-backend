-- +goose Up
-- +goose StatementBegin
ALTER TABLE giveaway_requirements
    ADD COLUMN IF NOT EXISTS account_age_min_year INT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE giveaway_requirements
    DROP COLUMN IF EXISTS account_age_min_year;
-- +goose StatementEnd

