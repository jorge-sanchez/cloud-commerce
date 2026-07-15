-- Migration 000001: create widgets table
-- Widgets are the example aggregate owned by a tenant. Replace with your
-- real domain when you fork this template.

CREATE TABLE IF NOT EXISTS widgets (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  UUID        NOT NULL,
    name       TEXT        NOT NULL,
    status     TEXT        NOT NULL DEFAULT 'draft',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT widgets_status_check CHECK (status IN ('draft', 'published', 'archived'))
);

CREATE INDEX IF NOT EXISTS idx_widgets_tenant_id ON widgets(tenant_id);
