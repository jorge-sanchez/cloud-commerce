-- Migration 000004: create product_images table
-- Images are a child collection of the Product aggregate (RFC-003): they are
-- attached/reordered/removed through the aggregate and persisted atomically.
-- The bytes live in object storage (GCS, ADR-013); this table holds only the
-- storage key + metadata. Public URLs are composed at read time from a
-- configured base so the serving host (direct GCS today, a CDN later) can move
-- without a data migration.

CREATE TABLE IF NOT EXISTS product_images (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID        NOT NULL,
    product_id   UUID        NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    variant_id   UUID,                          -- reserved for per-variant imagery; unused at launch
    storage_key  TEXT        NOT NULL,
    alt_text     TEXT        NOT NULL DEFAULT '',
    position     INT         NOT NULL,          -- 0 = primary/thumbnail
    content_type TEXT        NOT NULL,
    byte_size    BIGINT      NOT NULL CHECK (byte_size >= 0),
    width        INT         NOT NULL DEFAULT 0,
    height       INT         NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Tenant-led composite index (ADR-001); the collection is read per product in
-- position order.
CREATE INDEX IF NOT EXISTS idx_product_images_tenant_product_position
    ON product_images(tenant_id, product_id, position);

-- Storage keys are globally unique (content-addressed under a tenant prefix);
-- guard against a double-finalize of the same object.
CREATE UNIQUE INDEX IF NOT EXISTS idx_product_images_storage_key
    ON product_images(storage_key);
