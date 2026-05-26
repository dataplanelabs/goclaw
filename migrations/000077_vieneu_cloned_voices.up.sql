-- Migration: per-tenant VieNeu cloned voice registry.
--
-- Each row points at a WAV reference recording stored on disk under
-- <data_dir>/vieneu-refs/{tenant_id}/{id}.wav (the row id IS the on-disk
-- filename — refstore + DB are co-keyed). voice_id is "cloned:<id>" and is
-- what the LLM / dashboard passes back to the tts tool to invoke this voice.

CREATE TABLE vieneu_cloned_voices (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    voice_id    TEXT NOT NULL,
    ref_text    TEXT NOT NULL,
    name        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ,
    UNIQUE (tenant_id, voice_id)
);

CREATE INDEX idx_vieneu_cloned_voices_tenant
    ON vieneu_cloned_voices (tenant_id)
    WHERE deleted_at IS NULL;
