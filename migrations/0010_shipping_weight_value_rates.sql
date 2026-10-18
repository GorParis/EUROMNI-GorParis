ALTER TABLE variants
  ADD COLUMN weight_grams integer CHECK (weight_grams IS NULL OR weight_grams >= 0);

ALTER TABLE shipping_rates
  ADD COLUMN rate_mode text NOT NULL DEFAULT 'fixed'
    CHECK (rate_mode IN ('fixed', 'weight_value'));

CREATE TABLE shipping_rate_rules (
  shop_id uuid NOT NULL,
  carrier_code text NOT NULL,
  zone_code text NOT NULL,
  min_weight_grams bigint NOT NULL CHECK (min_weight_grams >= 0),
  max_weight_grams bigint CHECK (max_weight_grams IS NULL OR max_weight_grams > min_weight_grams),
  min_subtotal numeric(19, 4) NOT NULL DEFAULT 0 CHECK (min_subtotal >= 0),
  max_subtotal numeric(19, 4) CHECK (max_subtotal IS NULL OR max_subtotal > min_subtotal),
  amount numeric(19, 4) NOT NULL CHECK (amount >= 0),
  PRIMARY KEY (shop_id, carrier_code, zone_code, min_weight_grams, min_subtotal),
  FOREIGN KEY (shop_id, carrier_code, zone_code)
    REFERENCES shipping_rates (shop_id, carrier_code, zone_code) ON DELETE CASCADE
);

ALTER TABLE orders
  ADD COLUMN shipping_rate_mode text NOT NULL DEFAULT 'fixed',
  ADD COLUMN shipping_weight_grams bigint,
  ADD COLUMN shipping_pricing_subtotal numeric(19, 4),
  ADD COLUMN shipping_rate_rule jsonb;
