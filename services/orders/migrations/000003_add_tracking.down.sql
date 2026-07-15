-- Migration 000003 (down): remove tracking fields

ALTER TABLE orders DROP COLUMN IF EXISTS carrier;
ALTER TABLE orders DROP COLUMN IF EXISTS tracking_number;
