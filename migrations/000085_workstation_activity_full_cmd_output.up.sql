-- Migration 000085: add cmd_full and output_tail to workstation_activity.
-- cmd_full: full command, secrets redacted (no 200-char truncation).
-- output_tail: last ~8KB of combined stdout+stderr, secrets redacted.

ALTER TABLE workstation_activity ADD COLUMN IF NOT EXISTS cmd_full TEXT;
ALTER TABLE workstation_activity ADD COLUMN IF NOT EXISTS output_tail TEXT;
