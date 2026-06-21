ALTER TABLE cron_jobs ALTER COLUMN inject_target_history_limit SET DEFAULT 100;

UPDATE cron_jobs
SET inject_target_history_limit = 100
WHERE inject_target_history_limit = 50;
