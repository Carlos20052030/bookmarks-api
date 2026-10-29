-- Ensure key_hash column exists for bcrypt API key hashing
-- Safe to run multiple times (IF NOT EXISTS)
ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS key_hash VARCHAR(255);

-- If column existed but was too small, expand it
ALTER TABLE api_keys
    ALTER COLUMN key_hash TYPE VARCHAR(255);

-- Ensure it's NOT NULL for new records (existing NULLs allowed for migration)
-- Note: existing rows may have NULL, new rows must have hash
-- Application code handles this by always writing hash on create

-- Index for prefix-based lookups (used by bcrypt auth)
CREATE INDEX IF NOT EXISTS idx_api_keys_key_prefix ON api_keys(key_prefix);