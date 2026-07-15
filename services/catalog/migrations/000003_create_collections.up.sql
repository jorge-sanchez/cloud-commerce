-- Migration 000003: create collections and collection_products tables
-- Collections group products for storefront navigation (issue #9). The
-- handle is the URL slug, unique per tenant.

CREATE TABLE IF NOT EXISTS collections (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  UUID        NOT NULL,
    title      TEXT        NOT NULL,
    handle     TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Handles are the storefront URL key — unique per tenant (ADR-001: tenant leads).
CREATE UNIQUE INDEX IF NOT EXISTS idx_collections_tenant_handle ON collections(tenant_id, handle);
CREATE INDEX IF NOT EXISTS idx_collections_tenant_created ON collections(tenant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS collection_products (
    collection_id UUID        NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    product_id    UUID        NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    tenant_id     UUID        NOT NULL,
    added_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (collection_id, product_id)
);

CREATE INDEX IF NOT EXISTS idx_collection_products_tenant ON collection_products(tenant_id, product_id);
