-- Reverse migration 000004: drop product_images.

DROP INDEX IF EXISTS idx_product_images_storage_key;
DROP INDEX IF EXISTS idx_product_images_tenant_product_position;
DROP TABLE IF EXISTS product_images;
