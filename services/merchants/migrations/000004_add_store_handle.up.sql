-- Migration 000004: add public store handle (issue #15)
-- The handle is the storefront URL key, globally unique (it resolves the
-- tenant, so it cannot be tenant-scoped). Existing merchants are backfilled
-- from their name plus an id prefix to guarantee uniqueness.

ALTER TABLE merchants ADD COLUMN IF NOT EXISTS handle TEXT NOT NULL DEFAULT '';

UPDATE merchants
SET handle = trim(both '-' from regexp_replace(lower(name), '[^a-z0-9]+', '-', 'g'))
             || '-' || substr(id::text, 1, 4)
WHERE handle = '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_merchants_handle ON merchants(handle);
