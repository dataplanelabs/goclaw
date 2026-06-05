---
name: goclaw-agent-review
description: >-
  Review and improve a production GoClaw agent (persona) from its recent chat
  sessions. Pulls the agent's group/DM/cron sessions from the prod DB, analyzes
  the conversations for capability gaps, fabrication, tone/sycophancy, language
  leaks and tool failures, audits the agent's memory documents + knowledge graph
  for duplication/staleness/privacy leaks/test pollution, synthesizes concrete
  prompt + persona improvements, then applies them to the context files (DB AND
  the gcplane source-of-truth in goclaw-config) and cleans up memory/KG. Use when
  asked to "review agent X", "improve <persona> from its chats/last 24h",
  "audit the agent's memory/knowledge graph", "tune the SHTP/lan-runner agent",
  or to harden a live GoClaw persona after observing bad replies in a Zalo/
  Telegram group.
---

# GoClaw Agent Review & Improvement

Turn a GoClaw agent's real conversation history into concrete, verified
improvements to its prompt, persona, memory and knowledge graph. Built from the
2026-06-05 lan-runner/SHTP review (`worktrees/goclaw-skills/plans/260605-1023-lan-runner-shtp-review/`).

## When to use
- "Review the chats of agent X over the last 24h and improve it."
- "Audit agent X's memory / knowledge graph."
- The persona gave bad answers in a live group and you want a grounded fix.

## Mental model (read first)
- **Two sources of truth.** Agent context files (IDENTITY.md, SOUL.md, …) live in
  the `agent_context_files` table BUT the gcplane-managed ones are also defined in
  `goclaw-config/<tenant>/agents.yaml`. A gcplane reconcile **overwrites the DB
  from the config**. So a DB-only IDENTITY edit is temporary — you MUST mirror it
  into `agents.yaml` and bump `manifest.yaml` `metadata.environment`. Check which
  files are gcplane-managed: `grep -n "name: .*\.md" agents.yaml` (often only
  IDENTITY.md + SOUL.md; CAPABILITIES/AGENTS_*/USER are DB-only and persist).
- **Runtime data persists.** `memory_documents`, `memory_chunks`, `kg_entities`,
  `kg_relations` are NOT gcplane-managed — edits there stick.
- **Model traits vs prompt gaps.** Some failures (e.g. glm-5.1 substituting Han for
  Hán-Việt words, sycophancy, ignoring soft "run `date` first" gates) are model-level —
  a prompt rule helps but doesn't fully hold. Per-turn datetime injection is a good
  robust fix for the date gate. For the Han-substitution leak, use a PRECISE prompt
  rule (target *accidental* substitution; never forbid legitimate Chinese/Japanese
  output) — do NOT strip Han at the output layer: it limits real multilingual use
  (translation, etymology, quoting). A model switch is the only deeper fix. Flag
  runtime-code items for a goclaw PR.
- **Confirm before destructive/sensitive actions.** Deleting private data (the
  operator's banking/contract data often leaks into a public agent's `user_id IS
  NULL` global scope) and switching model/identity are the user's call — ask.

## Procedure

### 1. Prerequisites — prod DB access
See `references/db-access.md`. Short form (everest cluster):
```bash
cd <infra-repo> && eval "$(mise env -s zsh)"   # loads KUBECONFIG
PSQL() { kubectl exec -n databases goclaw-db-1-1 -c postgres -- psql -U postgres -d goclaw -X -t -A "$@"; }
PSQLi() { kubectl exec -i -n databases goclaw-db-1-1 -c postgres -- psql -U postgres -d goclaw -X -q "$@"; } # writes via stdin
```
Use `-U postgres` (peer auth); the app role needs a password over TCP.

### 2. Locate the agent + sessions
```sql
SELECT id, agent_key, display_name, model, tenant_id FROM agents WHERE display_name ILIKE '%<name>%' OR agent_key ILIKE '%<name>%';
-- ALL recent sessions for the agent — every group it belongs to + DMs + cron, not one group
SELECT session_key, channel, jsonb_array_length(messages) msgs, updated_at
FROM sessions WHERE agent_id = '<agent-uuid>' AND updated_at > now() - interval '7 days' ORDER BY updated_at DESC;
```
Review **all** of them (each group/DM has its own context); don't scope to a single
group unless the user explicitly asks. Skip `ws`/`http` sessions (web-playground/test);
focus on real messaging channels (`zalo-*`, telegram, etc.). Group names live in `channel_contacts.display_name` keyed by `sender_id` (the group id in the session_key `agent:<key>:<channel>:group:<id>`). Session `metadata->>'display_name'` = last sender, NOT the group name.

### 3. Extract to a working dir (`plans/<date>-<slug>/raw/`)
- **Sessions → readable transcripts:** dump `jsonb_array_elements(messages)` to JSONL per session, then `scripts/build_transcript.py` (handles the `[From: name (uid)]` user prefix, assistant tool_calls, security-wrapped tool results). Message shape: `role` user/assistant/tool, `content`, `created_at`, assistant `tool_calls[]`, tool `tool_call_id`.
- **Context files:** `agent_context_files` (agent-level) + `user_context_files` (per `user_id`, incl. `group:<channel>:<id>`). Agent-global memory scope = `user_id IS NULL`.
- **Memory:** `memory_documents` (path,user_id,content). **KG:** `kg_entities` (name,entity_type,description,confidence,user_id) + `kg_relations` (`source_entity_id`/`target_entity_id` — NOT from/to — relation_type, user_id).

### 4. Analyze (use a Workflow if ultracode/opted-in; else inline)
Read the primary transcript yourself, then fan out auditors + adversarial verify
(see `references/analysis-prompts.md`). Hunt for: fabrication under pressure +
fake citations; language leaks (Han chars in non-Chinese output — but PRESERVE
legitimately member-quoted Japanese/other-language text); summarize-vs-find-source
confusion; sycophancy; date/timezone misses; tool failures (browser eval, yt-dlp,
search ad-spam); permission friction. For memory/KG: duplication/fragmentation,
bloat, staleness, **test-scope pollution (`ws-test-*`)**, **privacy leaks**
(banking/contract/tax in the public agent's scope), cross-scope entity fragments.
Always have a verifier red-team the synthesis (it catches wrong counts, wrong
filenames, over-broad deletes, false UID collisions).

### 5. Apply context-file improvements (BOTH places)
1. Build the revised file locally; diff to confirm 0 lines removed.
2. DB: `UPDATE agent_context_files SET content=convert_from(decode('<base64>','base64'),'UTF8'), updated_at=now() WHERE agent_id=… AND file_name=…;` (base64 over stdin — robust for Vietnamese/large text). Per-group USER.md → `user_context_files … AND user_id='group:…'`.
3. **gcplane mirror:** edit the same content block in `goclaw-config/<tenant>/agents.yaml` (re-indent under `content: |`), bump `manifest.yaml` `metadata.environment`, commit + open a PR (no auto-merge on owned repos). Otherwise the DB edit reverts on reconcile.

### 6. Clean up memory + KG
Back up first: `SELECT json_agg(row_to_json(t)) FROM (SELECT * FROM <table> WHERE …) t;` per table (full rows incl. ids = restorable). Then, per `references/cleanup-sql.md`:
- **Privacy:** delete banking/contract/tax memory docs + KG entities/relations (ask first). Verify 0 residual markers.
- **Test scopes:** `DELETE … WHERE user_id IN ('ws-test-*')` from memory + KG. Check test-only names aren't real members first.
- **KG dedup/merge:** exact-name within-scope merge + cross-scope entity resolution — use the **collision-safe repoint** pattern (the unique constraint `(agent_id,user_id,source,relation_type,target)` rejects a naive UPDATE; dedup relations on the *mapped* canonical key first, then repoint). Prune off-domain transient nodes (news/price/media) but KEEP member persons + domain knowledge.
- **Memory consolidation:** merge fragmented files (e.g. member profiles) into one canonical doc; fix UID collisions; delete auto-extract/lesson spam; clean leaked CJK tokens. Re-chunk updated docs: `tsv` is a GENERATED column (auto from `text`), `embedding` is nullable + search is hybrid → tsv works immediately; note embedding backfill as follow-up. `memory_chunks` require `tenant_id` (NOT NULL) and cascade-delete with their document.

### 7. Verify + record
Re-run end-to-end checks: context lengths match, 0 private markers, 0 test scopes,
0 unresolved KG relations, CJK only in preserved member quotes. Write a report to
the plan dir and back up the touched rows. Note context changes take effect on next
session load; if `self_evolve:true`, your edits are the new evolution baseline.

## Gotchas (hard-won)
- gcplane reconcile reverts DB-only IDENTITY/SOUL edits — always mirror to config + bump environment.
- Han-char leak from Chinese-origin models (glm-5.1) lands in chat AND saved memory: the model renders Hán-Việt words as Han (研究 for "nghiên cứu"). Fix with a PRECISE prompt rule for accidental substitution — NOT an output-layer Han strip (that limits legitimate translation/etymology/quoting). "Don't limit the language."
- KG relation columns are `source_entity_id`/`target_entity_id`; unique constraint blocks naive merge repoints.
- Don't blanket-delete by keyword — verify real members (e.g. a person desc mentioning "football digest") aren't caught.
- Verifier always: it has caught wrong relation math, wrong filenames, false UID collisions, "zero-fallout" overstatements.
