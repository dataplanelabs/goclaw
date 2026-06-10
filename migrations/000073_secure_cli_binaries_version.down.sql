DROP INDEX IF EXISTS idx_secure_cli_binaries_version;
ALTER TABLE secure_cli_binaries DROP COLUMN IF EXISTS version;
