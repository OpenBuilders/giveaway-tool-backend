-- +goose Up
-- +goose StatementBegin
-- Add 'pending' value to giveaway_status enum
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_type t
        JOIN pg_enum e ON t.oid = e.enumtypid
        WHERE t.typname = 'giveaway_status' AND e.enumlabel = 'pending'
    ) THEN
        ALTER TYPE giveaway_status ADD VALUE 'pending';
    END IF;
END$$;

-- Extend giveaway_requirements.type allowed values to include 'boost' and 'custom'
ALTER TABLE giveaway_requirements
    DROP CONSTRAINT IF EXISTS giveaway_requirements_type_check,
    ADD CONSTRAINT giveaway_requirements_type_check CHECK (type IN ('subscription','boost','custom'));

-- Add name/description columns if not present (idempotent with later migration)
ALTER TABLE giveaway_requirements
    ADD COLUMN IF NOT EXISTS name TEXT NULL,
    ADD COLUMN IF NOT EXISTS description TEXT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Cannot remove enum value safely; leave as-is.
-- Narrow type check back to only 'subscription'
ALTER TABLE giveaway_requirements
    DROP CONSTRAINT IF EXISTS giveaway_requirements_type_check,
    ADD CONSTRAINT giveaway_requirements_type_check CHECK (type IN ('subscription'));
-- Optionally drop added columns
ALTER TABLE giveaway_requirements
    DROP COLUMN IF EXISTS name,
    DROP COLUMN IF EXISTS description;
-- +goose StatementEnd


