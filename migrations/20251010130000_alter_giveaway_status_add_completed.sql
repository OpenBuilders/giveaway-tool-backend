-- +goose Up
-- +goose StatementBegin
-- Add 'completed' value to giveaway_status enum if missing
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_type t
        JOIN pg_enum e ON t.oid = e.enumtypid
        WHERE t.typname = 'giveaway_status' AND e.enumlabel = 'completed'
    ) THEN
        ALTER TYPE giveaway_status ADD VALUE 'completed';
    END IF;
END$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- No safe downgrade for removing enum values; noop
-- +goose StatementEnd


