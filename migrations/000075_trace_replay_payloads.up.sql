-- Per-trace replay payload capture for retry-failed-trace feature.
-- Sibling to traces; keeps the hot list query untouched.

CREATE TABLE trace_replay_payloads (
    trace_id        UUID PRIMARY KEY REFERENCES traces(id) ON DELETE CASCADE,
    tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    session_key     TEXT NOT NULL,
    payload         JSONB,
    payload_version SMALLINT NOT NULL DEFAULT 1,
    oversize        BOOLEAN NOT NULL DEFAULT FALSE,
    byte_size       INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_replay_payloads_session ON trace_replay_payloads(session_key, created_at);
CREATE INDEX idx_replay_payloads_tenant ON trace_replay_payloads(tenant_id);

-- Short-lived (60s TTL) row blocking double-clicks on retry.
CREATE TABLE trace_retry_locks (
    trace_id   UUID PRIMARY KEY REFERENCES traces(id) ON DELETE CASCADE,
    tenant_id  UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    locked_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    locked_by  UUID NOT NULL
);

CREATE INDEX idx_retry_locks_expiry ON trace_retry_locks(locked_at);

-- True after the first successful channel.Send within this trace; surfaces
-- "this run already sent a message" warning at retry time.
ALTER TABLE traces ADD COLUMN outbound_emitted BOOLEAN NOT NULL DEFAULT FALSE;
