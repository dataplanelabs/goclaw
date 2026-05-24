-- Add source attribution for skill ownership tracking.
-- Default 'unknown' for legacy rows; new uploads stamp the actual source
-- (gcplane | cli | bundled | unknown). The upload handler refuses to overwrite
-- a source='gcplane' skill from a non-gcplane source unless force_imperative=true.
ALTER TABLE skills ADD COLUMN source TEXT NOT NULL DEFAULT 'unknown';
CREATE INDEX idx_skills_source ON skills (source);
