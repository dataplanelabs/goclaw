ALTER TABLE cron_jobs ADD COLUMN inject_target_history_limit integer NOT NULL DEFAULT 50;
