-- Migration 000001 (down): drop widgets table

DROP INDEX IF EXISTS idx_widgets_tenant_id;
DROP TABLE IF EXISTS widgets;
