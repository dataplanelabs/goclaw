-- Normalize vault_documents.path to tenant-root-relative (strip leading
-- tenants/<slug>/). The interceptor historically stored that prefix for
-- non-master tenants while rescan/upload stored bare paths, indexing the same
-- file under two tree roots. Dedupe before stripping: a prefixed row can
-- collide with a bare row under uq_vault_docs_agent_team_scope_path; keep the
-- latest updated_at. tsv + path_basename are GENERATED → recompute on UPDATE.

-- 1. Drop rows that would collide after stripping (keep latest per normalized key).
-- regexp_replace leaves non-prefixed paths unchanged, so it doubles as the
-- normalized key for ALL rows (equivalent to SQLite's CASE ELSE passthrough).
DELETE FROM vault_documents
WHERE id IN (
    SELECT id FROM (
        SELECT id,
               ROW_NUMBER() OVER (
                   PARTITION BY tenant_id,
                                COALESCE(agent_id, '00000000-0000-0000-0000-000000000000'),
                                COALESCE(team_id,  '00000000-0000-0000-0000-000000000000'),
                                scope,
                                regexp_replace(path, '^tenants/[^/]+/', '')
                   ORDER BY updated_at DESC, id DESC
               ) AS rn
        FROM vault_documents
    ) ranked
    WHERE rn > 1
);

-- 2. Strip the prefix. Two-slash gate leaves a degenerate single-segment
--    tenants/<slug> row (no file part) untouched.
UPDATE vault_documents
SET path = regexp_replace(path, '^tenants/[^/]+/', '')
WHERE path LIKE 'tenants/%/%';
