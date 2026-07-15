-- Migration 000003: processed-events dedupe (issue #18)
-- order_paid deliveries are at-least-once; a replayed deduction would
-- corrupt stock. The envelope ID is inserted in the same transaction as
-- the deduction — a conflict means "already applied".

CREATE TABLE IF NOT EXISTS processed_events (
    event_id     UUID        PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
