-- Migration 000003 (down): remove store settings columns

ALTER TABLE merchants DROP COLUMN IF EXISTS support_email;
ALTER TABLE merchants DROP COLUMN IF EXISTS timezone;
ALTER TABLE merchants DROP COLUMN IF EXISTS currency;
