-- Migration 000004 (down): remove store handle

DROP INDEX IF EXISTS idx_merchants_handle;
ALTER TABLE merchants DROP COLUMN IF EXISTS handle;
