# Cleanup SQL patterns (verified 2026-06-05)

All run as `BEGIN; … COMMIT;` with a residual-check SELECT before COMMIT. Back up
affected rows first (`SELECT json_agg(row_to_json(t)) FROM (SELECT * FROM <t> WHERE …) t;`).

## Privacy deletion (memory)
```sql
CREATE TEMP TABLE _d AS SELECT id FROM memory_documents WHERE agent_id=:aid AND (
  path='memory/<banking-file>.md' OR (path='<dreaming>.md' AND user_id='<scope>'));
DELETE FROM memory_chunks WHERE document_id IN (SELECT id FROM _d);
DELETE FROM memory_documents WHERE id IN (SELECT id FROM _d);
-- verify: 0 rows match account-number / contract markers
```

## Privacy deletion (KG) — match business markers, exclude generic + club entities
```sql
CREATE TEMP TABLE _priv AS SELECT id FROM kg_entities WHERE tenant_id=:tid AND name<>'Zalo'
  AND (name ~* '(Citibank|INV-ABS|TechcomLife|Huly|DPL-|Independent Contractor|invoice|insurance|…)'
    OR description ~* '(…)');
DELETE FROM kg_relations WHERE tenant_id=:tid AND (source_entity_id IN (SELECT id FROM _priv) OR target_entity_id IN (SELECT id FROM _priv));
DELETE FROM kg_entities WHERE id IN (SELECT id FROM _priv);
```

## Test-scope deletion
```sql
DELETE FROM kg_relations  WHERE tenant_id=:tid AND user_id IN ('ws-test-runner-900000001','ws-test-shtp-900000009');
DELETE FROM kg_entities   WHERE tenant_id=:tid AND user_id IN (…);
DELETE FROM memory_chunks    WHERE agent_id=:aid AND user_id IN (…);
DELETE FROM memory_documents WHERE agent_id=:aid AND user_id IN (…);
-- first: check test-only names don't carry a real member's only KG record
```

## Collision-safe entity merge (exact-name dedup OR cross-scope resolution)
The unique constraint rejects a naive `UPDATE source_entity_id=keep`. Dedup the
relations on their *mapped* canonical key BEFORE repointing:
```sql
-- _map(dup_id, keep_id): for exact dedup use a window:
CREATE TEMP TABLE _canon AS SELECT id,
  first_value(id) OVER (PARTITION BY name, coalesce(user_id,'') ORDER BY updated_at DESC, created_at DESC, id) keep_id
  FROM kg_entities WHERE tenant_id=:tid;
CREATE TEMP TABLE _map AS SELECT id dup_id, keep_id FROM _canon WHERE id<>keep_id;
-- survivors: one relation per (agent,user,mapped_source,rel,mapped_target), no self-loops
CREATE TEMP TABLE _keep_rel AS
  SELECT DISTINCT ON (agent_id, coalesce(user_id,''), new_source, relation_type, new_target) id
  FROM (SELECT r.id,r.agent_id,r.user_id,r.relation_type,r.created_at,
          coalesce(ms.keep_id,r.source_entity_id) new_source, coalesce(mt.keep_id,r.target_entity_id) new_target
        FROM kg_relations r LEFT JOIN _map ms ON r.source_entity_id=ms.dup_id LEFT JOIN _map mt ON r.target_entity_id=mt.dup_id
        WHERE r.tenant_id=:tid) x
  WHERE new_source<>new_target
  ORDER BY agent_id, coalesce(user_id,''), new_source, relation_type, new_target, created_at;
DELETE FROM kg_relations WHERE tenant_id=:tid AND id NOT IN (SELECT id FROM _keep_rel)
  AND (source_entity_id IN (SELECT dup_id FROM _map) OR target_entity_id IN (SELECT dup_id FROM _map));
UPDATE kg_relations r SET source_entity_id=m.keep_id FROM _map m WHERE r.source_entity_id=m.dup_id AND r.tenant_id=:tid;
UPDATE kg_relations r SET target_entity_id=m.keep_id FROM _map m WHERE r.target_entity_id=m.dup_id AND r.tenant_id=:tid;
DELETE FROM kg_entities WHERE id IN (SELECT dup_id FROM _map);
-- always verify: 0 unresolved relations (every source/target still exists)
```
For cross-scope person resolution, hand-build `_map` with explicit dup→canonical
ids (confirm same person first), then run the same survivor/repoint block.

## Re-chunk an updated memory doc (tsv search; embedding backfills later)
`tsv` is GENERATED from `text`; `embedding` nullable. INSERT chunks must set
`tenant_id`. See `scripts/build_transcript.py` sibling pattern in the lan-runner
plan dir (`generate_members_sql.py`) for section-based chunking + base64 INSERTs.
