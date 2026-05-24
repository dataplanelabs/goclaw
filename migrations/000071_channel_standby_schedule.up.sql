ALTER TABLE channel_instances
    ADD COLUMN silence_schedule JSONB DEFAULT NULL;

CREATE TABLE channel_thread_schedules (
    channel_instance_id UUID NOT NULL REFERENCES channel_instances(id) ON DELETE CASCADE,
    thread_key TEXT NOT NULL,
    schedule JSONB NOT NULL,
    expires_at TIMESTAMPTZ,
    reason TEXT DEFAULT '',
    created_by VARCHAR(255) DEFAULT '',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (channel_instance_id, thread_key)
);

CREATE INDEX idx_channel_thread_schedules_expires
    ON channel_thread_schedules(expires_at)
    WHERE expires_at IS NOT NULL;
