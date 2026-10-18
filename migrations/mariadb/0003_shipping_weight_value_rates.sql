ALTER TABLE variants
  ADD COLUMN IF NOT EXISTS weight_grams integer NULL CHECK (weight_grams IS NULL OR weight_grams >= 0);

ALTER TABLE shipping_rates
  ADD COLUMN IF NOT EXISTS rate_mode VARCHAR(32) NOT NULL DEFAULT 'fixed';

CREATE TABLE IF NOT EXISTS shipping_rate_rules (
  shop_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  carrier_code VARCHAR(64) NOT NULL,
  zone_code VARCHAR(64) NOT NULL,
  min_weight_grams BIGINT NOT NULL CHECK (min_weight_grams >= 0),
  max_weight_grams BIGINT NULL,
  min_subtotal numeric(19, 4) NOT NULL DEFAULT 0 CHECK (min_subtotal >= 0),
  max_subtotal numeric(19, 4) NULL,
  amount numeric(19, 4) NOT NULL CHECK (amount >= 0),
  PRIMARY KEY (shop_id, carrier_code, zone_code, min_weight_grams, min_subtotal),
  CONSTRAINT fk_shipping_rate_rules_rate
    FOREIGN KEY (shop_id, carrier_code, zone_code)
    REFERENCES shipping_rates (shop_id, carrier_code, zone_code) ON DELETE CASCADE,
  CHECK (max_weight_grams IS NULL OR max_weight_grams > min_weight_grams),
  CHECK (max_subtotal IS NULL OR max_subtotal > min_subtotal)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

ALTER TABLE orders
  ADD COLUMN IF NOT EXISTS shipping_rate_mode VARCHAR(32) NOT NULL DEFAULT 'fixed',
  ADD COLUMN IF NOT EXISTS shipping_weight_grams BIGINT NULL,
  ADD COLUMN IF NOT EXISTS shipping_pricing_subtotal numeric(19, 4) NULL,
  ADD COLUMN IF NOT EXISTS shipping_rate_rule JSON NULL;
