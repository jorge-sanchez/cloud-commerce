-- Migration 000002: create outbox table (ADR-002)
-- Domain events are written here in the same transaction as the state change
-- they describe. The relay drains undelivered rows in insertion order and
-- marks them delivered; delivery is at-least-once, so consumers dedupe on id.

CREATE TABLE IF NOT EXISTS outbox (
    position     BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    id           UUID        NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    tenant_id    UUID        NOT NULL,
    aggregate_id UUID        NOT NULL,
    event_type   TEXT        NOT NULL,
    occurred_at  TIMESTAMPTZ NOT NULL,
    payload      JSONB       NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivered_at TIMESTAMPTZ
);

-- Relay scan: undelivered rows in insertion order. Partial so it stays small
-- as delivered rows accumulate. The outbox is platform infrastructure scanned
-- across tenants by the relay, so the tenant_id-leading index rule (ADR-001)
-- does not apply here.
CREATE INDEX IF NOT EXISTS idx_outbox_undelivered
    ON outbox (position) WHERE delivered_at IS NULL;
