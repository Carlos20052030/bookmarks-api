-- Fix key_prefix column length to accommodate the generated prefix format
-- "nuntix_live_" (12 chars) + 8 hex chars = 20 chars
-- Using VARCHAR(32) for safety margin
ALTER TABLE api_keys ALTER COLUMN key_prefix TYPE VARCHAR(32);

-- Update indexes if needed (they should auto-update)
-- But explicitly recreate for clarity
DROP INDEX IF EXISTS idx_api_keys_key_prefix;
CREATE INDEX idx_api_keys_key_prefix ON api_keys(key_prefix);