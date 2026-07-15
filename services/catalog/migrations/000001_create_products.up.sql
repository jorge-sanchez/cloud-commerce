-- Migration 000001: create products and variants tables
-- Product is the aggregate root; variants are always written with it
-- (SaveNewWithVariants). Prices are integer minor units; display currency
-- comes from the merchant's store settings.

CREATE TABLE IF NOT EXISTS products (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID        NOT NULL,
    title       TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    status      TEXT        NOT NULL DEFAULT 'draft',
    options     JSONB       NOT NULL DEFAULT '[]',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT products_status_check CHECK (status IN ('draft', 'active', 'archived'))
);

-- Tenant-led composite index (ADR-001); list is newest-first.
CREATE INDEX IF NOT EXISTS idx_products_tenant_created ON products(tenant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS variants (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id    UUID        NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    tenant_id     UUID        NOT NULL,
    sku           TEXT        NOT NULL,
    option_values JSONB       NOT NULL DEFAULT '[]',
    price_cents   BIGINT      NOT NULL CHECK (price_cents >= 0),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- SKUs are unique per tenant (not just per product) so fulfillment and POS
-- can key on them. Tenant leads (ADR-001).
CREATE UNIQUE INDEX IF NOT EXISTS idx_variants_tenant_sku ON variants(tenant_id, lower(sku));
CREATE INDEX IF NOT EXISTS idx_variants_tenant_product ON variants(tenant_id, product_id);
