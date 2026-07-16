-- Migration 000002: outbound webhooks (issue #44)
-- Merchants register URLs; every orders.* event is POSTed with an HMAC
-- signature. Deliveries are deduped per (event, endpoint).

CREATE TABLE IF NOT EXISTS webhook_endpoints (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  UUID        NOT NULL,
    url        TEXT        NOT NULL,
    secret     TEXT        NOT NULL,
    active     BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_webhook_endpoints_tenant
    ON webhook_endpoints(tenant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    event_id     UUID        NOT NULL,
    endpoint_id  UUID        NOT NULL REFERENCES webhook_endpoints(id) ON DELETE CASCADE,
    delivered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (event_id, endpoint_id)
);
