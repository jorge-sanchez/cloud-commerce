-- Migration 000001: create carts and orders tables
-- Carts are anonymous: the unguessable cart id is the buyer's capability
-- (documented ADR-001 exception — buyers have no tenant claim; the cart row
-- carries the tenant). Items snapshot price/title at add time so later
-- catalog edits never change a cart or an order.

CREATE TABLE IF NOT EXISTS carts (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  UUID        NOT NULL,
    currency   TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_carts_tenant ON carts(tenant_id, created_at);

CREATE TABLE IF NOT EXISTS cart_items (
    cart_id     UUID   NOT NULL REFERENCES carts(id) ON DELETE CASCADE,
    variant_id  UUID   NOT NULL,
    tenant_id   UUID   NOT NULL,
    sku         TEXT   NOT NULL,
    title       TEXT   NOT NULL,
    price_cents BIGINT NOT NULL CHECK (price_cents >= 0),
    qty         BIGINT NOT NULL CHECK (qty > 0),

    PRIMARY KEY (cart_id, variant_id)
);

CREATE TABLE IF NOT EXISTS orders (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    number      BIGINT      GENERATED ALWAYS AS IDENTITY,
    tenant_id   UUID        NOT NULL,
    email       TEXT        NOT NULL,
    currency    TEXT        NOT NULL,
    total_cents BIGINT      NOT NULL CHECK (total_cents >= 0),
    status      TEXT        NOT NULL DEFAULT 'pending',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT orders_status_check CHECK (status IN ('pending', 'paid', 'fulfilled', 'cancelled'))
);

CREATE INDEX IF NOT EXISTS idx_orders_tenant_created ON orders(tenant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS order_items (
    order_id    UUID   NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    variant_id  UUID   NOT NULL,
    tenant_id   UUID   NOT NULL,
    sku         TEXT   NOT NULL,
    title       TEXT   NOT NULL,
    price_cents BIGINT NOT NULL CHECK (price_cents >= 0),
    qty         BIGINT NOT NULL CHECK (qty > 0),

    PRIMARY KEY (order_id, variant_id)
);
