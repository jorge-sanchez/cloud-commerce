-- Migration 000006: merchant-defined flat shipping methods (RFC-001/ADR-011)
-- Every existing tenant is seeded a free Standard method: checkout requires
-- at least one active method (RFC-001 resolution).

CREATE TABLE IF NOT EXISTS shipping_methods (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID        NOT NULL,
    name        TEXT        NOT NULL,
    price_cents BIGINT      NOT NULL CHECK (price_cents >= 0),
    active      BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_shipping_methods_tenant
    ON shipping_methods(tenant_id, active, created_at);

INSERT INTO shipping_methods (tenant_id, name, price_cents)
SELECT id, 'Standard', 0 FROM merchants
ON CONFLICT DO NOTHING;
