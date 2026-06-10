ALTER TABLE secure_cli_binaries ADD COLUMN version TEXT;
CREATE INDEX idx_secure_cli_binaries_version ON secure_cli_binaries (version) WHERE version IS NOT NULL;
