-- +goose Up
-- Add wallet_address to users
ALTER TABLE users ADD COLUMN IF NOT EXISTS wallet_address TEXT;
CREATE INDEX IF NOT EXISTS idx_users_wallet_address ON users ((lower(wallet_address)));

-- +goose Down
DROP INDEX IF EXISTS idx_users_wallet_address;
ALTER TABLE users DROP COLUMN IF EXISTS wallet_address;

