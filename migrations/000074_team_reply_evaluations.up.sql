CREATE TABLE team_reply_evaluations (
    id                     UUID PRIMARY KEY DEFAULT uuid_generate_v7(),
    channel_instance_id    UUID NOT NULL REFERENCES channel_instances(id) ON DELETE CASCADE,
    tenant_id              UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    thread_key             TEXT NOT NULL,
    session_key            TEXT NOT NULL,
    team_msg_id            TEXT NOT NULL,
    captured_at            TIMESTAMPTZ NOT NULL,
    customer_message       TEXT NOT NULL DEFAULT '',
    team_reply             TEXT NOT NULL,
    hypothesized_bot_reply TEXT,
    diff_score             REAL,
    diff_reasoning         TEXT,
    judge_agent_key        TEXT,
    judge_model            TEXT,
    judge_provider         TEXT,
    judge_latency_ms       INTEGER,
    judge_error            TEXT,
    judge_completed_at     TIMESTAMPTZ,
    created_at             TIMESTAMPTZ DEFAULT NOW(),
    updated_at             TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (channel_instance_id, team_msg_id)
);

CREATE INDEX idx_team_reply_evals_tenant_time
    ON team_reply_evaluations(tenant_id, captured_at DESC);
CREATE INDEX idx_team_reply_evals_channel_time
    ON team_reply_evaluations(channel_instance_id, captured_at DESC);
CREATE INDEX idx_team_reply_evals_thread
    ON team_reply_evaluations(channel_instance_id, thread_key, captured_at DESC);
CREATE INDEX idx_team_reply_evals_pending_judge
    ON team_reply_evaluations(captured_at)
    WHERE judge_completed_at IS NULL AND judge_error IS NULL;
