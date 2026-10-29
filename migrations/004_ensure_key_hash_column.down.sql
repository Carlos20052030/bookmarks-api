-- Down migration for 004_ensure_key_hash_column.up.sql
DROP INDEX IF EXISTS idx_api_keys_key_prefix;
ALTER TABLE api_keys ALTER COLUMN key_hash TYPE VARCHAR(64);