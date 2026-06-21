ALTER TABLE cron_jobs ALTER COLUMN inject_target_history_limit SET DEFAULT 50;

UPDATE cron_jobs
SET inject_target_history_limit = 50
WHERE inject_target_history_limit = 100;
