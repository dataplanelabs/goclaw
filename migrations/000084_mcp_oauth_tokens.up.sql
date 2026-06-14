CREATE TABLE mcp_oauth_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id   UUID NOT NULL REFERENCES mcp_servers(id) ON DELETE CASCADE,
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id     TEXT,

    access_token      TEXT NOT NULL,
    refresh_token     TEXT,
    token_type        VARCHAR(20) NOT NULL DEFAULT 'Bearer',
    scopes            TEXT,
    expires_at        TIMESTAMPTZ,
    issued_at         TIMESTAMPTZ,

    dcr_client_id     TEXT NOT NULL DEFAULT '',
    dcr_client_secret TEXT,
    dcr_issuer        TEXT NOT NULL DEFAULT '',

    token_endpoint    TEXT NOT NULL DEFAULT '',
    resource_uri      TEXT,

    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX mcp_oauth_tokens_global_uq
    ON mcp_oauth_tokens (server_id, tenant_id)
    WHERE user_id IS NULL;

CREATE UNIQUE INDEX mcp_oauth_tokens_user_uq
    ON mcp_oauth_tokens (server_id, tenant_id, user_id)
    WHERE user_id IS NOT NULL;

CREATE INDEX mcp_oauth_tokens_server_tenant ON mcp_oauth_tokens (server_id, tenant_id);
