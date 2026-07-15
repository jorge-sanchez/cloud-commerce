-- Migration 000003: fulfillment tracking fields (issue #27)

ALTER TABLE orders ADD COLUMN IF NOT EXISTS tracking_number TEXT NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN IF NOT EXISTS carrier         TEXT NOT NULL DEFAULT '';
