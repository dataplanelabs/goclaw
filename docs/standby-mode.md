# Agent Standby Mode

## Overview

Standby mode is a declarative-schedule feature on `channel_instances` (with per-thread overrides) that gates message processing at the pipeline entry. While standby resolves true for a `(tenant, channel, thread)` triplet, the agent **still observes and writes to working/episodic memory**, but does NOT call the LLM, run tools, or emit replies. Outcome: ~$0 LLM cost during silent windows, full context continuity when the window expires.

Three use cases shaped the design:

1. **Always-passive transcript collector** — channel default `standby`; agent observes 24/7 and a cron-fired delegate hands off a daily summary to a reporting agent.
2. **Shift handoff** — channel `active` overnight + weekends, `standby` during human work hours.
3. **Ad-hoc pause** — agent self-invokes `enter_standby(duration_seconds=7200, reason="lunch")` for the current thread.

## Architecture

```text
Inbound message
      │
      ▼
┌──────────────────┐
│   StandbyGate    │  ← first iteration stage (internal/pipeline/standby_gate.go)
│  resolves Mode   │
└────────┬─────────┘
         │
   Mode == standby?
         │
   ┌─────┴─────┐
   │ yes       │ no
   ▼           ▼
AbortRun     Think → Tool → Observe → Checkpoint
   │           │
   └─────┬─────┘
         ▼
   FinalizeStage  ← always runs (memory writes preserved)
         │
         ▼
   channel.Send (skipped when StandbyMode set — FinalContent is "")
```

Schedules live in `channel_instances.silence_schedule` (JSONB) + `channel_thread_schedules` (per-thread overrides). The pipeline reads them through an in-memory `ScheduleRegistry` with a 60-second TTL cache and push-reload on writes (`internal/channels/schedule/registry.go`).

## Schedule schema

```json
{
  "default_mode": "active",
  "windows": [
    {
      "id": "human-hours",
      "mode": "standby",
      "tz": "Asia/Saigon",
      "weekday": "mon-fri",
      "start": "09:00",
      "end": "17:00"
    },
    {
      "id": "vacation-2026-06",
      "mode": "standby",
      "from": "2026-06-15T00:00:00+07:00",
      "until": "2026-06-22T00:00:00+07:00"
    }
  ]
}
```

Field reference:

| Field | Type | Meaning |
|---|---|---|
| `default_mode` | `"active" \| "standby"` | Fallback when no window matches. Empty = `active`. |
| `windows[].mode` | `"active" \| "standby"` | What the window resolves to when matched. Empty = `standby`. |
| `windows[].tz` | IANA timezone | Defaults to `Asia/Saigon`. |
| `windows[].weekday` | `mon` \| `mon,wed,fri` \| `mon-fri` | Recurring window day spec. List + range cannot mix. |
| `windows[].start`/`end` | `15:04` | Recurring window range. Supports cross-midnight (`22:00-06:00`). |
| `windows[].from`/`until` | RFC3339 timestamp | One-shot window. |

A window must have **either** recurring (`weekday + start + end`) **or** one-shot (`from + until`) — mixing is rejected at validate time.

## Precedence rules

Load-bearing. Operators MUST understand these:

1. **One-shot windows beat recurring** on overlap.
2. Within each tier, **first matching window wins**.
3. No match → falls back to `default_mode`.
4. **Per-thread override REPLACES instance default** for that thread — no merge.

## Three example configs

### Example 1: Always-passive transcript collector (UC1)

```json
{ "default_mode": "standby" }
```

Pair with a cron job — see "Cron summary handoff" below.

### Example 2: Shift handoff (UC2)

```json
{
  "default_mode": "active",
  "windows": [{
    "id": "human-hours",
    "mode": "standby",
    "weekday": "mon-fri",
    "start": "09:00",
    "end": "17:00",
    "tz": "Asia/Saigon"
  }]
}
```

Bot replies overnight + weekends; silent during human work hours.

### Example 3: Ad-hoc pause (UC3)

Operator-set:

```json
{
  "default_mode": "active",
  "windows": [{
    "id": "vacation-2026-06",
    "mode": "standby",
    "from": "2026-06-15T00:00:00+07:00",
    "until": "2026-06-22T00:00:00+07:00"
  }]
}
```

Agent-set (one tool call):

```text
enter_standby(duration_seconds=7200, reason="lunch break")
```

Writes a per-thread one-shot window with `expires_at = now + duration`.

## Cron summary handoff pattern (UC1)

`internal/cron/` + the `delegate` tool are ~80% wired today — no new code needed. Create a `cron_job`:

```json
{
  "name": "daily-standby-summary",
  "schedule_kind": "cron",
  "cron_expression": "0 18 * * *",
  "timezone": "Asia/Saigon",
  "payload": {
    "kind": "agent_turn",
    "message": "Summarize the last 24h of conversation from this channel, then call delegate(agent_key='reporting-agent', payload={summary: <summary>}) to hand off to the reporting agent."
  },
  "deliver": false
}
```

Fires at 18:00 daily; the agent reads recent memory (already populated by standby-mode pipeline writes), summarizes, and uses `delegate` for handoff. The cron-fired outbound bypasses the gate by design (cron-initiated traffic is intentional).

## Per-thread overrides

- Set via `channels.thread_schedule_set` RPC or the `enter_standby` tool.
- REPLACES instance schedule for that thread.
- One-shot windows with `expires_at` are auto-cleaned by `PurgeExpiredThreadSchedules`. v1 ships without a cleanup cron — call manually or schedule one if needed.
- Useful for: "this admin group always replies even when bot is in shift-handoff mode", or "snooze this DM for 2 hours."

## Tool reference

`enter_standby(duration_seconds: int, reason?: string)` — agent self-pauses the current thread. Requires channel context (not callable from CLI). Duration range: 60–86400 seconds.

## WS RPC reference

| Method | Params | Returns | Guard |
|---|---|---|---|
| `channels.schedule_get` | `channel_instance_id` | `Schedule \| null` | tenant member |
| `channels.schedule_set` | `channel_instance_id, schedule` | `ok` | tenant admin |
| `channels.schedule_delete` | `channel_instance_id` | `ok` | tenant admin |
| `channels.thread_schedule_list` | `channel_instance_id` | `[]ThreadSchedule` | tenant member |
| `channels.thread_schedule_get` | `channel_instance_id, thread_key` | `ThreadSchedule \| null` | tenant member |
| `channels.thread_schedule_set` | `channel_instance_id, thread_key, schedule, expires_at?, reason?` | `ok` | tenant admin |
| `channels.thread_schedule_delete` | `channel_instance_id, thread_key` | `ok` | tenant admin |

All params are snake_case. Cross-tenant access returns `NOT_FOUND` (no info leak). See `internal/gateway/methods/channel_schedules.go`.

## Thread-key format

The canonical thread-key shape, shared between the gate (read) and `enter_standby` (write):

- DM: `direct:{peerID}`
- Group: `group:{chatID}`

Defined once in `internal/pipeline/standby_gate.go::BuildStandbyThreadKey`. Both write paths (tool, RPC) MUST use this format or schedules silently won't match.

## Operational notes

- Schedule edits propagate within ≤60s (TTL cache) and immediately on the editing pod (push reload).
- Memory writes are preserved during standby — agent retains context when the window expires.
- LLM calls and tool runs are skipped — zero token spend during silent windows.
- Cron-fired outbound bypasses standby (intentional traffic). Synthetic events (e.g. Zalo reactions) ARE gated.
- Channel rename/delete must call `registry.InvalidateInstance(tenantID, channelName)` to flush the name→id cache.

## Known limitations (v1)

- No calendar visualizer in the web UI — raw JSON only.
- No escalation-keyword regex bypass during standby ("soft silence" — deferred).
- No multi-gateway hot-reload via `pg_notify` — single-gateway assumption.

## References

- Plan: `plans/260524-0741-agent-standby-mode/`
- Brainstorm brief: `plans/reports/brainstorm-260524-0717-passive-observer-mode.md`
- Decisions / non-goals: `plans/260524-0741-agent-standby-mode/decisions.md`
- Code: `internal/channels/schedule/`, `internal/pipeline/standby_gate.go`, `internal/store/{pg,sqlitestore}/channel_schedules.go`, `internal/gateway/methods/channel_schedules*.go`, `internal/tools/enter_standby.go`, `ui/web/src/pages/channels/channel-detail/channel-standby-tab.tsx`
