-- Migration 000007: store country, tax mode, and merchant tax rates
-- (RFC-002/ADR-012). Existing stores default to exclusive; tax_mode is
-- near-immutable once orders exist (no update API — support operation).

ALTER TABLE merchants ADD COLUMN IF NOT EXISTS country  TEXT NOT NULL DEFAULT '';
ALTER TABLE merchants ADD COLUMN IF NOT EXISTS tax_mode TEXT NOT NULL DEFAULT 'exclusive';
ALTER TABLE merchants DROP CONSTRAINT IF EXISTS merchants_tax_mode_check;
ALTER TABLE merchants ADD CONSTRAINT merchants_tax_mode_check
    CHECK (tax_mode IN ('inclusive', 'exclusive'));

CREATE TABLE IF NOT EXISTS tax_rates (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID        NOT NULL,
    name                TEXT        NOT NULL,
    country             TEXT        NOT NULL,
    region              TEXT        NOT NULL DEFAULT '',
    rate_bps            INT         NOT NULL CHECK (rate_bps >= 0 AND rate_bps <= 10000),
    applies_to_shipping BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tax_rates_tenant ON tax_rates(tenant_id, country, region);
