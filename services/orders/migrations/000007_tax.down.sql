-- Migration 000007 (down): remove tax fields

ALTER TABLE carts DROP COLUMN IF EXISTS tax_inclusive;
ALTER TABLE orders DROP COLUMN IF EXISTS tax_inclusive;
ALTER TABLE orders DROP COLUMN IF EXISTS tax_rate_bps;
ALTER TABLE orders DROP COLUMN IF EXISTS tax_name;
ALTER TABLE orders DROP COLUMN IF EXISTS tax_cents;
