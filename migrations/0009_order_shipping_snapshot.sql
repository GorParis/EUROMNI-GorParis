ALTER TABLE orders
  ADD COLUMN shipping_carrier_code text,
  ADD COLUMN shipping_carrier_name text,
  ADD COLUMN shipping_zone_code text,
  ADD COLUMN shipping_zone_name text;
