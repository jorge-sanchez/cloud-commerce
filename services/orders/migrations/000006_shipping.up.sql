-- Migration 000006: shipping address, method, and cost on orders (RFC-001)
-- Address is a structured snapshot like prices; POS sales carry the
-- registering location instead (RFC-001 acceptance resolution).

ALTER TABLE orders ADD COLUMN IF NOT EXISTS ship_name       TEXT NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN IF NOT EXISTS ship_line1      TEXT NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN IF NOT EXISTS ship_line2      TEXT NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN IF NOT EXISTS ship_city       TEXT NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN IF NOT EXISTS ship_region     TEXT NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN IF NOT EXISTS ship_postal     TEXT NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN IF NOT EXISTS ship_country    TEXT NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN IF NOT EXISTS ship_phone      TEXT NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN IF NOT EXISTS shipping_method TEXT NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN IF NOT EXISTS shipping_cents  BIGINT NOT NULL DEFAULT 0 CHECK (shipping_cents >= 0);
ALTER TABLE orders ADD COLUMN IF NOT EXISTS location_id     UUID;
