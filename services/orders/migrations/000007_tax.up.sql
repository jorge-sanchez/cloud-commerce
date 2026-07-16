-- Migration 000007: tax snapshot on orders; carts remember the store's
-- tax mode at creation (RFC-002).

ALTER TABLE orders ADD COLUMN IF NOT EXISTS tax_cents     BIGINT  NOT NULL DEFAULT 0 CHECK (tax_cents >= 0);
ALTER TABLE orders ADD COLUMN IF NOT EXISTS tax_name      TEXT    NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN IF NOT EXISTS tax_rate_bps  INT     NOT NULL DEFAULT 0;
ALTER TABLE orders ADD COLUMN IF NOT EXISTS tax_inclusive BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE carts  ADD COLUMN IF NOT EXISTS tax_inclusive BOOLEAN NOT NULL DEFAULT FALSE;
