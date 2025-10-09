-- +goose Up
-- +goose StatementBegin
-- 1) Drop existing primary key (likely on (giveaway_id, place)) to allow place to become nullable
ALTER TABLE giveaway_prizes DROP CONSTRAINT IF EXISTS giveaway_prizes_pkey;

-- 2) Ensure surrogate key column exists and is populated
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'giveaway_prizes' AND column_name = 'id'
  ) THEN
    ALTER TABLE giveaway_prizes ADD COLUMN id BIGSERIAL;
  END IF;
END$$;

-- Ensure default is set for future inserts
DO $$
DECLARE seq regclass;
BEGIN
  seq := pg_get_serial_sequence('giveaway_prizes', 'id');
  IF seq IS NULL THEN
    -- Create a sequence if not present and attach as default
    IF NOT EXISTS (
      SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
      WHERE c.relkind = 'S' AND c.relname = 'giveaway_prizes_id_seq' AND n.nspname = 'public'
    ) THEN
      EXECUTE 'CREATE SEQUENCE giveaway_prizes_id_seq';
    END IF;
    ALTER TABLE giveaway_prizes ALTER COLUMN id SET DEFAULT nextval('giveaway_prizes_id_seq');
  ELSE
    EXECUTE format('ALTER TABLE giveaway_prizes ALTER COLUMN id SET DEFAULT nextval(%L)', seq::text);
  END IF;
END$$;

-- Backfill ids for existing rows
UPDATE giveaway_prizes SET id = COALESCE(id, nextval(pg_get_serial_sequence('giveaway_prizes','id')))
WHERE id IS NULL;

-- 3) Establish primary key on surrogate id
ALTER TABLE giveaway_prizes ADD PRIMARY KEY (id);

-- 4) Now place can be nullable; also add quantity column
ALTER TABLE giveaway_prizes
    ALTER COLUMN place DROP NOT NULL,
    ADD COLUMN IF NOT EXISTS quantity INT NOT NULL DEFAULT 1;

-- 5) Ensure only one prize per fixed place per giveaway
CREATE UNIQUE INDEX IF NOT EXISTS giveaway_prizes_unique_place
  ON giveaway_prizes (giveaway_id, place)
  WHERE place IS NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS giveaway_prizes_unique_place;
ALTER TABLE giveaway_prizes DROP CONSTRAINT IF EXISTS giveaway_prizes_pkey;
-- Recreate previous composite PK (may fail if null places exist; for down migration simplicity we skip strict restoration)
ALTER TABLE giveaway_prizes ADD PRIMARY KEY (giveaway_id, place);
ALTER TABLE giveaway_prizes
    ALTER COLUMN place SET NOT NULL,
    DROP COLUMN IF EXISTS quantity,
    DROP COLUMN IF EXISTS id;
-- +goose StatementEnd
