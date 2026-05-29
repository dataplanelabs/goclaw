-- Irreversible: once stripped, bare rescan rows and stripped prefixed rows are
-- indistinguishable, so the tenants/<slug>/ prefix cannot be reconstructed for
-- only the rows that originally had it. The interceptor (Phase 2) now writes
-- bare paths regardless, so a re-prefix would diverge from live behavior anyway.
SELECT 1;
