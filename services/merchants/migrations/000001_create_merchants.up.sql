-- Migration 000001: create merchants and merchant_users tables
-- A merchant IS the tenant: merchants.id is the tenant_id carried by every
-- token (ADR-006) and every other service's rows (ADR-001).

CREATE TABLE IF NOT EXISTS merchants (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT        NOT NULL,
    status     TEXT        NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT merchants_status_check CHECK (status IN ('active', 'suspended'))
);

CREATE TABLE IF NOT EXISTS merchant_users (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id   UUID        NOT NULL REFERENCES merchants(id),
    email         TEXT        NOT NULL,
    password_hash TEXT        NOT NULL,
    role          TEXT        NOT NULL DEFAULT 'owner',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT merchant_users_role_check CHECK (role IN ('owner', 'staff'))
);

-- Login looks users up by email before any tenant is known — this global
-- unique index is the documented exception to tenant-led indexing (ADR-001),
-- like the outbox. Emails are stored lowercased by the service.
CREATE UNIQUE INDEX IF NOT EXISTS idx_merchant_users_email ON merchant_users(email);

-- Tenant-scoped listing of a merchant's users (ADR-001: tenant leads).
CREATE INDEX IF NOT EXISTS idx_merchant_users_merchant_id ON merchant_users(merchant_id, id);
