-- Migration 000001: sent-notification log (issue #29)
-- One row per delivered email; event_id UNIQUE is the at-least-once dedupe
-- (check → send → record; a crash between send and record means a rare
-- duplicate email, which beats a lost one).

CREATE TABLE IF NOT EXISTS notifications (
    id        UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id  UUID        NOT NULL UNIQUE,
    tenant_id UUID        NOT NULL,
    order_id  UUID        NOT NULL,
    kind      TEXT        NOT NULL,
    recipient TEXT        NOT NULL,
    subject   TEXT        NOT NULL,
    sent_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notifications_tenant ON notifications(tenant_id, sent_at DESC);
