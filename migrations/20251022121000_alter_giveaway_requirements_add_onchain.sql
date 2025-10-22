-- Add on-chain requirement fields for TON and Jetton checks
ALTER TABLE giveaway_requirements
    ADD COLUMN IF NOT EXISTS ton_min_balance_nano BIGINT,
    ADD COLUMN IF NOT EXISTS jetton_address TEXT,
    ADD COLUMN IF NOT EXISTS jetton_min_amount BIGINT;


