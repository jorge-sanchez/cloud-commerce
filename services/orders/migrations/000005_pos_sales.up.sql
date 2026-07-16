-- Migration 000005: POS sale idempotency (issue #38, ADR-010)
-- Offline POS clients retry queued sales; the client-generated sale id
-- makes the endpoint idempotent.

ALTER TABLE orders ADD COLUMN IF NOT EXISTS client_sale_id TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_client_sale
    ON orders (tenant_id, client_sale_id) WHERE client_sale_id IS NOT NULL;
