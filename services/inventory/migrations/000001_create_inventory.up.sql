-- Migration 000001: create locations and stock_levels tables
-- Stock is tracked per (variant, location). Rows are initialized at zero by
-- the catalog.product_created consumer; sku is denormalized from that event
-- so inventory reads never call the catalog service.

CREATE TABLE IF NOT EXISTS locations (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  UUID        NOT NULL,
    name       TEXT        NOT NULL,
    is_default BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Exactly one default location per tenant (ADR-001: tenant leads).
CREATE UNIQUE INDEX IF NOT EXISTS idx_locations_tenant_default
    ON locations(tenant_id) WHERE is_default;
CREATE INDEX IF NOT EXISTS idx_locations_tenant ON locations(tenant_id, created_at);

CREATE TABLE IF NOT EXISTS stock_levels (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID        NOT NULL,
    location_id UUID        NOT NULL REFERENCES locations(id),
    variant_id  UUID        NOT NULL,
    sku         TEXT        NOT NULL,
    on_hand     BIGINT      NOT NULL DEFAULT 0 CHECK (on_hand >= 0),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- One row per variant per location; the at-least-once consumer relies
    -- on this for idempotent initialization (ON CONFLICT DO NOTHING).
    CONSTRAINT stock_levels_unique UNIQUE (tenant_id, location_id, variant_id)
);

CREATE INDEX IF NOT EXISTS idx_stock_levels_tenant_sku ON stock_levels(tenant_id, sku);
