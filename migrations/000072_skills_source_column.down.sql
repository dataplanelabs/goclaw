-- Idempotent rollback. Source attribution is metadata-only; dropping it loses
-- the gcplane-managed marker so server-side overwrite gating becomes a no-op
-- (everything reverts to 'unknown'). No data loss to user-facing skill content.
DROP INDEX IF EXISTS idx_skills_source;
ALTER TABLE skills DROP COLUMN IF EXISTS source;
