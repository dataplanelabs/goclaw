# Team-Reply Capture + Evaluation (Zalo OA)

Captures every human-team-typed reply on a Zalo Official Account, persists
it with provenance metadata into the bot's conversation history, runs an
optional "judge" agent that produces a hypothesized bot reply + diff
score, and exposes per-tenant analytics + a JSONL export for fine-tuning.

Primary use case: bot in standby on a channel handled by humans — capture
team replies, grade them against what the bot would have said, export the
divergent examples as training data.

---

## Architecture

```
                                Zalo OA Manager app
                              (human team types reply)
                                        │
                                        ▼
                       /onbehalf/conversation polled by
                        PollWorker (60s tick by default)
                                        │
                  ┌─────────────────────┴─────────────────────┐
                  │ for each new OA-side message              │
                  ▼                                           ▼
        sessions.messages JSONB                  team_reply_evaluations row
        (assistant + metadata.source="team")     (judge fields NULL)
                  │                                           │
                  │                                           │
                  └────────────► team.reply.observed ◄────────┘
                                        │
                                        ▼
                          JudgeWorker (consolidation
                          subscriber, runs in
                          eventbus worker pool)
                                        │
                                        ▼
                      agent.Router.Get(judgeAgentID).Run()
                                        │
                                        ▼
                       UpdateJudgeVerdict(score, hypo, reasoning)
                                        │
                                        ▼
                Web UI "Team Analytics" tab + JSONL export
```

Capture uses polling because Zalo OA webhook delivery for human-typed
sends in the Manager app is undocumented + empirically NO-GO in our
verification (see `plans/260524-1050-team-reply-capture-eval/research-zalo-oa-webhook-capture-260524.md`).
Polling endpoints (`/onbehalf/listrecentchat`, `/onbehalf/conversation`)
return the full conversation regardless of source — confirmed in
`ttpro1995/zalo-python-sdk`.

---

## Enabling capture (operator steps)

1. Channels → `{zalo-oa-channel}` → **Team Analytics** tab.
2. Toggle **Capture team replies** ON.
3. (Optional) Toggle **Auto-judge captures** ON + paste the judge agent's
   `agent_key` (e.g. `team-reply-judge`).
4. Click **Save**.
5. Restart the channel so the new poll worker picks up the toggle
   (current Phase 4 caveat: toggle requires channel restart). The web UI
   surfaces this as a hint on toggle save.

The first poll tick runs immediately; subsequent ticks every 60s by
default (override per-channel via `config.team_reply_poll_interval_seconds`).

---

## Choosing a judge agent

Recommended: dedicate a separate agent (don't reuse the customer-facing
bot — its SOUL.md is wrong for grading work).

Sample SOUL.md for a judge agent:

```markdown
You evaluate customer-support replies. For each request you will receive:
- A customer message
- The reply a human team member actually sent

Your job:
1. Write what an expert support assistant *would* have replied (be
   concise + actionable).
2. Compare to the team reply on a 0.0–1.0 similarity scale.
3. Note the key difference in 1–2 sentences.

Respond ONLY with valid JSON in the shape requested in the user prompt.
Do not add prose or markdown.
```

Recommended model: same family/size as your production bot. Judging
gpt-5.5 output with claude-haiku is apples-to-oranges.

---

## Diff score interpretation

| Range | Meaning | Training value |
|---|---|---|
| 0.0 – 0.3 | Team reply diverges significantly | **Highest** — bot would have said something materially different |
| 0.3 – 0.7 | Same intent, different wording/structure | Good for tone-tuning |
| 0.7 – 1.0 | Essentially the same | Low — model already does this |

Use `max_diff_score=0.5` when exporting JSONL to cherry-pick the most
informative examples.

---

## JSONL export for fine-tuning

Click **Export training data (JSONL)** on the Team Analytics tab. Format
is OpenAI chat-completions training shape:

```jsonl
{"messages":[{"role":"user","content":"where is order #42"},{"role":"assistant","content":"Order #42 ships today. Sorry for the delay!"}]}
{"messages":[{"role":"user","content":"can I cancel"},{"role":"assistant","content":"Yes — within 1 hour of ordering. Let me know the order # and I'll process it."}]}
```

System prompt is intentionally omitted from each row — operators prepend
a uniform system line at fine-tuning time via `jq`:

```bash
jq -c '. + {messages: ([{role:"system",content:"You are a helpful support assistant."}] + .messages)}' \
  team-replies-zalo-oa-annhien-2026-05-24.jsonl
```

Filters available:
- `max_diff_score` — include only divergent examples
- `since` / `until` — time-bound batches
- `include_pending` — include rows whose judge hasn't completed (default off)

Server-side cap: 5000 rows or 5MB per call. Narrow the filter if you hit
this.

---

## Cost model

| Step | Cost | Notes |
|---|---|---|
| Capture | free | DB writes only; polling already runs |
| Judge call | ~500–1000 tokens in + ~300–500 out per evaluation | One LLM call per captured reply |

Example: 100 team replies/day × gpt-4o-mini ($0.15/1M in, $0.60/1M out)
≈ **$0.05/day per channel**. Cheap.

Rate limit: 10 evals/min/tenant (5 burst). Adjustable via
`tenants.settings.judge_rate_limit` (not yet exposed in UI; edit via
`tenants.update` RPC).

---

## Operational notes

- **Memory continuity:** captured team replies become part of the bot's
  conversation history. When the bot reactivates (e.g. team hands off),
  it sees what was said.
- **Bot-API vs human distinction (v1 limitation):** the polling worker
  captures every OA-side message — including the bot's own API sends —
  as `source="team"`. The judge worker dedups via the `(channel, msg_id)`
  UNIQUE constraint on `team_reply_evaluations`, so each message is
  evaluated at most once. False-positive evaluations of bot replies are
  acceptable for v1. v2 will add an outbox correlation to distinguish.
- **`-118` (refresh token dead):** if the worker logs
  `oa.poll_worker.refresh_token_invalid`, the operator must re-consent
  the OA via the Credentials tab. Polling pauses until then.
- **Single-pod assumption:** in-memory polling cursor + per-pod rate
  limits. Multi-gateway = separate concern (same as the standby
  registry).
- **History retention:** Zalo's `/onbehalf/conversation` retention horizon
  is undocumented. After downtime longer than the horizon, captures in
  that window are lost. Test empirically; document if you hit it.
- **`sessions.messages` JSONB row size:** monitor; consider a compaction
  job at 100k+ messages/session.

---

## Troubleshooting

| Symptom | Likely cause |
|---|---|
| Captures appear, judges never run | Judge agent unconfigured, agent_key typo, or `consolidation.Register` ran without `pgStores.TeamReplyEvals` |
| `judge_error = "no_judge_agent_configured"` | Tenant.settings.judge_agent_key empty AND no per-channel override |
| `judge_error = "judge_agent_unavailable: ..."` | agent_key doesn't resolve in `agents` table — check spelling |
| `judge_error = "judge_parse_error: ..."` | Judge LLM returned non-JSON — pick a stronger judge model or strengthen the prompt |
| No captures showing | `capture_team_replies=false`, channel not restarted after toggle, or `-118` refresh-token error in logs |
| All captures show `source="team"` even bot's own sends | Expected in v1 — see "Bot-API vs human distinction" above |

---

## References

- Plan dir: `plans/260524-1050-team-reply-capture-eval/`
- Decisions + trade-offs: `plans/260524-1050-team-reply-capture-eval/decisions.md`
- Webhook research (why we picked polling): `plans/260524-1050-team-reply-capture-eval/research-zalo-oa-webhook-capture-260524.md`
- Standby mode pairing: `docs/standby-mode.md`
- Zalo OA docs: https://developers.zalo.me/docs/official-account/webhook/tong-quan
- Migration: PG 000074, SQLite slot 44 (`RequiredSchemaVersion=74`, `SchemaVersion=44`)
