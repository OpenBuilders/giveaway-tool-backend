-- +goose Up
-- +goose StatementBegin
-- Add pending status to enum
ALTER TYPE giveaway_status ADD VALUE IF NOT EXISTS 'pending';

-- Extend requirements table for custom requirements
ALTER TABLE giveaway_requirements
    ADD COLUMN IF NOT EXISTS title TEXT,
    ADD COLUMN IF NOT EXISTS description TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Down migration note: removing enum values is non-trivial; skipping
ALTER TABLE giveaway_requirements
    DROP COLUMN IF EXISTS title,
    DROP COLUMN IF EXISTS description;
-- +goose StatementEnd
