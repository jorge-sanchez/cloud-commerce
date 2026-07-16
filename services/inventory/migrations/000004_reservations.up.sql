-- Migration 000004: stock reservations (issue #37)
-- A reservation holds order quantities between checkout and payment:
-- created on order_placed (TTL), committed on order_paid (deducts on_hand),
-- released by the expiry sweep. UNIQUE(order_id) + processed_events give
-- at-least-once safety.

CREATE TABLE IF NOT EXISTS reservations (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  UUID        NOT NULL,
    order_id   UUID        NOT NULL UNIQUE,
    status     TEXT        NOT NULL DEFAULT 'active',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT reservations_status_check CHECK (status IN ('active', 'committed', 'released'))
);

CREATE INDEX IF NOT EXISTS idx_reservations_active_expiry
    ON reservations (expires_at) WHERE status = 'active';
CREATE INDEX IF NOT EXISTS idx_reservations_tenant ON reservations(tenant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS reservation_items (
    reservation_id UUID   NOT NULL REFERENCES reservations(id) ON DELETE CASCADE,
    variant_id     UUID   NOT NULL,
    tenant_id      UUID   NOT NULL,
    qty            BIGINT NOT NULL CHECK (qty > 0),

    PRIMARY KEY (reservation_id, variant_id)
);
