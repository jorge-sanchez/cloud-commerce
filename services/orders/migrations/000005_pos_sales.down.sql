-- Migration 000005 (down): remove POS idempotency

DROP INDEX IF EXISTS idx_orders_client_sale;
ALTER TABLE orders DROP COLUMN IF EXISTS client_sale_id;
