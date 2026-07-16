-- Migration 000007 (down): remove tax config

DROP TABLE IF EXISTS tax_rates;
ALTER TABLE merchants DROP CONSTRAINT IF EXISTS merchants_tax_mode_check;
ALTER TABLE merchants DROP COLUMN IF EXISTS tax_mode;
ALTER TABLE merchants DROP COLUMN IF EXISTS country;
