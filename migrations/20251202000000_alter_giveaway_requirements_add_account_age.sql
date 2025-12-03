-- +goose Up
-- +goose StatementBegin
ALTER TABLE giveaway_requirements
    ADD COLUMN IF NOT EXISTS account_age_max_year INT;

ALTER TABLE giveaway_requirements
    DROP CONSTRAINT IF EXISTS giveaway_requirements_type_check;

ALTER TABLE giveaway_requirements
    ADD CONSTRAINT giveaway_requirements_type_check CHECK (type IN ('subscription','boost','custom','premium','holdton','holdjetton','account_age'));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE giveaway_requirements
    DROP COLUMN IF EXISTS account_age_max_year;

ALTER TABLE giveaway_requirements
    DROP CONSTRAINT IF EXISTS giveaway_requirements_type_check;

ALTER TABLE giveaway_requirements
    ADD CONSTRAINT giveaway_requirements_type_check CHECK (type IN ('subscription','boost','custom','premium','holdton','holdjetton'));
-- +goose StatementEnd

