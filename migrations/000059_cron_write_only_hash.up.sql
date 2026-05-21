-- Migration: add write_only_hash column to cron_jobs.
--
-- Background: gcplane (the GitOps reconciler) treats CronJob.message and
-- CronJob.agentKey as "write-only" — the goclaw cron.list response does not
-- include them, so gcplane cannot detect drift in those fields by direct
-- comparison. The recommended pattern (already supported by gcplane via
-- reconciler.WriteOnlyHashField) is: gcplane sends a stable hash of the
-- write-only fields on every create/update, goclaw stores it as opaque
-- bytes and echoes it back in list/get responses. gcplane then compares
-- desired_hash (recomputed from manifest) against observed_hash from the
-- list response — drift detected without exposing the underlying values.
--
-- See: https://github.com/dataplanelabs/gcplane/issues/9
--
-- The column is opaque: goclaw never inspects or validates the value, it
-- only stores and returns it. NOT NULL DEFAULT '' so existing rows are
-- valid (empty string means "no hash recorded yet" — gcplane treats that
-- the same as a hash mismatch and re-applies on next reconcile).

ALTER TABLE cron_jobs ADD COLUMN write_only_hash TEXT NOT NULL DEFAULT '';
