-- Migration 000005: third-party API keys (issue #39)
-- The key itself is shown once and stored only as a SHA-256 hash; a key
-- exchanges for a short-lived platform JWT, so every existing authed API
-- works for integrations unchanged.

CREATE TABLE IF NOT EXISTS api_keys (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  UUID        NOT NULL,
    name       TEXT        NOT NULL,
    key_hash   TEXT        NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_api_keys_tenant ON api_keys(tenant_id, created_at DESC);
