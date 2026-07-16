-- Migration 000006 (down): remove shipping fields

ALTER TABLE orders DROP COLUMN IF EXISTS location_id;
ALTER TABLE orders DROP COLUMN IF EXISTS shipping_cents;
ALTER TABLE orders DROP COLUMN IF EXISTS shipping_method;
ALTER TABLE orders DROP COLUMN IF EXISTS ship_phone;
ALTER TABLE orders DROP COLUMN IF EXISTS ship_country;
ALTER TABLE orders DROP COLUMN IF EXISTS ship_postal;
ALTER TABLE orders DROP COLUMN IF EXISTS ship_region;
ALTER TABLE orders DROP COLUMN IF EXISTS ship_city;
ALTER TABLE orders DROP COLUMN IF EXISTS ship_line2;
ALTER TABLE orders DROP COLUMN IF EXISTS ship_line1;
ALTER TABLE orders DROP COLUMN IF EXISTS ship_name;
