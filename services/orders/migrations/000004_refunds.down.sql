-- Migration 000004 (down): remove refund support

ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_status_check;
ALTER TABLE orders ADD CONSTRAINT orders_status_check CHECK (status IN ('pending', 'paid', 'fulfilled', 'cancelled'));
ALTER TABLE orders DROP COLUMN IF EXISTS payment_reference;
