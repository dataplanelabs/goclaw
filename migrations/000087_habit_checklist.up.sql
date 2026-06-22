-- Per-user/day habit checklist: deterministic "is task X done today?" state that
-- the single coach-dispatcher cron gates on (replaces speculative per-task/retry crons).
CREATE TABLE habit_checklist_entries (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    tenant_id       UUID NOT NULL,
    agent_id        UUID NOT NULL,
    user_id         TEXT NOT NULL,        -- per-user or group:* scope key
    plan_date       DATE NOT NULL,        -- local calendar day (user TZ), the "today" axis
    task_key        VARCHAR(80) NOT NULL, -- stable slug: guzheng|piano|run|english|...
    title           VARCHAR(200) NOT NULL,
    scheduled_local VARCHAR(5),           -- "HH:MM" local clock; NULL = anytime-today
    status          VARCHAR(16) NOT NULL DEFAULT 'pending', -- pending|done|skipped
    nudge_count     INT NOT NULL DEFAULT 0,                 -- escalation = still pending after N ticks
    last_nudged_at  TIMESTAMPTZ,                            -- cadence floor
    completed_at    TIMESTAMPTZ,
    completion_note TEXT,                                   -- the user's confirmation phrase (audit)
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, agent_id, user_id, plan_date, task_key)
);

CREATE INDEX idx_habit_checklist_gate
    ON habit_checklist_entries (tenant_id, agent_id, user_id, plan_date, status);
