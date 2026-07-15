-- Migration 000003: add store settings columns to merchants
-- Settings are part of the Merchant aggregate (issue #1): currency and
-- timezone drive storefront display; support_email is shown to buyers.

ALTER TABLE merchants ADD COLUMN IF NOT EXISTS currency      TEXT NOT NULL DEFAULT 'USD';
ALTER TABLE merchants ADD COLUMN IF NOT EXISTS timezone      TEXT NOT NULL DEFAULT 'UTC';
ALTER TABLE merchants ADD COLUMN IF NOT EXISTS support_email TEXT NOT NULL DEFAULT '';
