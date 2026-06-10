DROP INDEX IF EXISTS idx_channel_thread_schedules_expires;
DROP TABLE IF EXISTS channel_thread_schedules;
ALTER TABLE channel_instances DROP COLUMN IF EXISTS silence_schedule;
