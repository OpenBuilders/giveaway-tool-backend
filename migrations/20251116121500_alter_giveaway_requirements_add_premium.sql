-- +goose Up
-- +goose StatementBegin
ALTER TABLE giveaway_requirements
    DROP CONSTRAINT IF EXISTS giveaway_requirements_type_check,
    ADD CONSTRAINT giveaway_requirements_type_check CHECK (type IN ('subscription','boost','custom','holdton','holdjetton','premium'));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE giveaway_requirements
    DROP CONSTRAINT IF EXISTS giveaway_requirements_type_check,
    ADD CONSTRAINT giveaway_requirements_type_check CHECK (type IN ('subscription','boost','custom','holdton','holdjetton'));
-- +goose StatementEnd


