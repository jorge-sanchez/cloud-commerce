-- Migration 000004: refund support (issue #28)
-- payment_reference stores the provider's payment id (set when the order
-- is marked paid) so refunds can address the original charge.

ALTER TABLE orders ADD COLUMN IF NOT EXISTS payment_reference TEXT NOT NULL DEFAULT '';
ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_status_check;
ALTER TABLE orders ADD CONSTRAINT orders_status_check
    CHECK (status IN ('pending', 'paid', 'fulfilled', 'cancelled', 'refunded'));
