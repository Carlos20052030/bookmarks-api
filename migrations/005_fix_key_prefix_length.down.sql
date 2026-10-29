-- Down migration for 005_fix_key_prefix_length.up.sql
DROP INDEX IF EXISTS idx_api_keys_key_prefix;
ALTER TABLE api_keys ALTER COLUMN key_prefix TYPE VARCHAR(16);
CREATE INDEX idx_api_keys_key_prefix ON api_keys(key_prefix);