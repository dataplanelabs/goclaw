ALTER TABLE traces DROP COLUMN IF EXISTS outbound_emitted;
DROP TABLE IF EXISTS trace_retry_locks;
DROP TABLE IF EXISTS trace_replay_payloads;
