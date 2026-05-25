-- backfill-team-reply-session-keys.sql
--
-- Idempotent migration: convert team_reply_evaluations.session_key from the
-- legacy "zalo_oa:<uid>" format to the canonical
-- "agent:<agent_key>:<channel_name>:direct:<uid>" format, AND merge any
-- messages stored in the legacy "zalo_oa:<uid>" session into the canonical
-- agent session so the operator sees one unified conversation history.
--
-- Safe to re-run: the WHERE session_key LIKE 'zalo_oa:%' filter skips rows
-- already migrated. The session-message merge dedups by (created_at, content)
-- so re-running does not duplicate any message.
--
-- Usage (prod):
--   kubectl exec -n databases goclaw-db-1-1 -c postgres -- \
--     psql -U goclaw -d goclaw \
--     -f /tmp/backfill-team-reply-session-keys.sql
--
-- Or paste into a psql session connected as `goclaw`.

BEGIN;

-- Step 1 — compute the canonical key for every legacy-keyed eval row.
-- legacy.thread_key is "direct:<uid>"; split_part extracts uid.
WITH legacy AS (
    SELECT e.id              AS eval_id,
           e.session_key     AS old_key,
           e.thread_key,
           e.channel_instance_id,
           ci.name           AS channel_name,
           a.agent_key,
           'agent:' || a.agent_key || ':' || ci.name || ':' || e.thread_key AS new_key
    FROM   team_reply_evaluations e
    JOIN   channel_instances ci ON ci.id = e.channel_instance_id
    JOIN   agents            a  ON a.id       = ci.agent_id
    WHERE  e.session_key LIKE 'zalo_oa:%'
),

-- Step 2 — for every (old_key → new_key) pair, append non-duplicate messages
-- from the legacy session into the canonical session.
-- Dedup key is (created_at, content) so re-runs are no-ops.
appended AS (
    UPDATE sessions tgt
    SET    messages = COALESCE(tgt.messages, '[]'::jsonb) || COALESCE((
               SELECT jsonb_agg(elem ORDER BY elem->>'created_at')
               FROM   sessions src
               CROSS JOIN LATERAL jsonb_array_elements(src.messages) elem
               WHERE  src.session_key = pair.old_key
                 AND  NOT EXISTS (
                     SELECT 1
                     FROM   jsonb_array_elements(COALESCE(tgt.messages, '[]'::jsonb)) tgt_elem
                     WHERE  tgt_elem->>'created_at' = elem->>'created_at'
                       AND  tgt_elem->>'content'    = elem->>'content'
                 )
           ), '[]'::jsonb)
    FROM   (
        SELECT DISTINCT old_key, new_key
        FROM   legacy
    ) AS pair
    WHERE  tgt.session_key = pair.new_key
    RETURNING tgt.session_key
)

-- Step 3 — flip team_reply_evaluations.session_key to canonical.
UPDATE team_reply_evaluations e
SET    session_key = l.new_key
FROM   legacy l
WHERE  e.id = l.eval_id
  AND  e.session_key LIKE 'zalo_oa:%';

-- Sanity reports — operator should see 0 rows on a second run.
SELECT 'remaining_legacy_evals' AS metric,
       count(*)                  AS value
FROM   team_reply_evaluations
WHERE  session_key LIKE 'zalo_oa:%';

SELECT 'orphan_legacy_sessions' AS metric,
       count(*)                  AS value
FROM   sessions
WHERE  session_key LIKE 'zalo_oa:%';

COMMIT;
