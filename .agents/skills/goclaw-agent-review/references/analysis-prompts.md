# Analysis workflow prompts

Fan-out (parallel auditors) → synthesis → adversarial verify. Use the Workflow
tool when ultracode/opted-in; otherwise run inline / a few Agent calls.

## Shared context block (prepend to every auditor)
> Auditing a PRODUCTION GoClaw agent "<persona>" (agent_key <key>), a <language>
> assistant for <community>, serving <channel/group>. Model: <model> (note if
> Chinese-origin → expect Han-char leakage). It does <domain tasks> AND general
> Q&A. Read the given files, report structured findings with concrete evidence
> (line refs/quotes/file+scope). Distinguish prompt-fixable from model-level.

## Auditors (one each, parallel)
1. **Conversation** — read the primary group transcript. Hunt: fabrication +
   fake citations (cite the exact line); accidental language leaks (Han rendered
   for Hán-Việt words) → fix with a PRECISE prompt rule, never an output-layer
   strip (that limits legitimate translation/quoting); summarize-vs-find-source
   confusion; sycophancy ("dạ anh đúng"); date/timezone misses; tool failures
   (browser eval syntax, missing yt-dlp, search ad-spam); permission friction;
   and what it did WELL.
2. **Config/persona** — read all context files + agent row. Empty stubs; rules
   that EXIST but were ignored (→ strengthen or move to a runtime guard); narrow
   persona vs general-assistant usage; which files are gcplane-managed.
3. **Memory audit** — `memory_documents.jsonl`. Duplication/fragmentation; bloat;
   staleness; **test pollution (`ws-test-*`)**; **privacy leaks** (banking/
   contract/tax in a public agent's `user_id IS NULL` scope); over-saving.
4. **KG audit** — entities + relations jsonl. Test pollution; dup/near-dup
   entities (same person ×N names/scopes); off-domain transient nodes; type
   consistency; orphan/unresolved relations.
5. **Secondary** — cron + internal sessions: proactive jobs, fabrication in
   digests (gold/price/news after 404s), public shaming on private data.

## Synthesis (1 agent, schema)
Consolidate into: executive_summary; prioritized_issues (rank, severity,
root_cause, fix-LAYER prompt|runtime|data|policy); context_file_edits (file,
section, change_type, **production-ready text in the agent's language**, rationale);
memory_cleanup[]; kg_cleanup[]; model_recommendation; open_questions.
Rules: terse native-language edits; prefer strengthening existing sections over
new ones; hoist ONE domain-agnostic anti-fabrication/anti-sycophancy/no-leak block
to the TOP (primacy) instead of restating per-domain rules.

## Adversarial verifier (1 agent, MANDATORY)
Red-team the synthesis vs the raw files: every edit must map to a real observed
failure; flag edits that over-restrict/break working behavior/bloat; verify
counts + filenames + scopes are correct; confirm cleanup deletes only genuine
test/private data; check a model-level claim (leak) really needs a runtime fix.
Past catches: wrong relation math, wrong target filename, false UID collision,
"zero-fallout" overstatement, naive CJK filter clobbering member Japanese.
