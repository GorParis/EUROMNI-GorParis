CREATE TABLE shipping_services (
  shop_id uuid NOT NULL,
  carrier_code text NOT NULL,
  code text NOT NULL CHECK (code ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
  name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 120),
  active boolean NOT NULL DEFAULT true,
  PRIMARY KEY (shop_id, carrier_code, code),
  FOREIGN KEY (shop_id, carrier_code)
    REFERENCES shipping_carriers (shop_id, code) ON DELETE CASCADE
);

INSERT INTO shipping_services (shop_id, carrier_code, code, name, active)
SELECT DISTINCT rate.shop_id, rate.carrier_code, 'legacy', 'Offre historique', carrier.active
FROM shipping_rates rate
JOIN shipping_carriers carrier
  ON carrier.shop_id = rate.shop_id AND carrier.code = rate.carrier_code;

ALTER TABLE shipping_rates
  ADD COLUMN service_code text NOT NULL DEFAULT 'legacy',
  ADD COLUMN rate_version uuid NOT NULL DEFAULT gen_random_uuid();

ALTER TABLE shipping_rate_rules
  ADD COLUMN service_code text NOT NULL DEFAULT 'legacy';

ALTER TABLE shipping_rate_rules
  DROP CONSTRAINT shipping_rate_rules_shop_id_carrier_code_zone_code_fkey;

ALTER TABLE shipping_rates
  DROP CONSTRAINT shipping_rates_pkey,
  ADD PRIMARY KEY (shop_id, carrier_code, service_code, zone_code),
  ADD CONSTRAINT shipping_rates_service_fkey
    FOREIGN KEY (shop_id, carrier_code, service_code)
    REFERENCES shipping_services (shop_id, carrier_code, code) ON DELETE CASCADE;

ALTER TABLE shipping_rate_rules
  DROP CONSTRAINT shipping_rate_rules_pkey,
  ADD PRIMARY KEY (shop_id, carrier_code, service_code, zone_code, min_weight_grams, min_subtotal),
  ADD CONSTRAINT shipping_rate_rules_rate_fkey
    FOREIGN KEY (shop_id, carrier_code, service_code, zone_code)
    REFERENCES shipping_rates (shop_id, carrier_code, service_code, zone_code) ON DELETE CASCADE;

CREATE TABLE shipping_rate_versions (
  rate_version uuid PRIMARY KEY,
  shop_id uuid NOT NULL,
  carrier_code text NOT NULL,
  service_code text NOT NULL,
  service_name text NOT NULL,
  zone_code text NOT NULL,
  rate_mode text NOT NULL CHECK (rate_mode IN ('fixed', 'weight_value')),
  amount numeric(19, 4) NOT NULL CHECK (amount >= 0),
  currency char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
  active boolean NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX shipping_rate_versions_lookup_idx
  ON shipping_rate_versions (shop_id, carrier_code, service_code, zone_code, created_at DESC);

CREATE TABLE shipping_rate_version_rules (
  rate_version uuid NOT NULL REFERENCES shipping_rate_versions (rate_version) ON DELETE CASCADE,
  min_weight_grams bigint NOT NULL CHECK (min_weight_grams >= 0),
  max_weight_grams bigint CHECK (max_weight_grams IS NULL OR max_weight_grams > min_weight_grams),
  min_subtotal numeric(19, 4) NOT NULL DEFAULT 0 CHECK (min_subtotal >= 0),
  max_subtotal numeric(19, 4) CHECK (max_subtotal IS NULL OR max_subtotal > min_subtotal),
  amount numeric(19, 4) NOT NULL CHECK (amount >= 0),
  PRIMARY KEY (rate_version, min_weight_grams, min_subtotal)
);

INSERT INTO shipping_rate_versions (
  rate_version, shop_id, carrier_code, service_code, service_name, zone_code,
  rate_mode, amount, currency, active
)
SELECT rate.rate_version, rate.shop_id, rate.carrier_code, rate.service_code,
       service.name, rate.zone_code, rate.rate_mode, rate.amount, rate.currency, rate.active
FROM shipping_rates rate
JOIN shipping_services service
  ON service.shop_id = rate.shop_id AND service.carrier_code = rate.carrier_code
 AND service.code = rate.service_code;

INSERT INTO shipping_rate_version_rules (
  rate_version, min_weight_grams, max_weight_grams, min_subtotal, max_subtotal, amount
)
SELECT rate.rate_version, rule.min_weight_grams, rule.max_weight_grams,
       rule.min_subtotal, rule.max_subtotal, rule.amount
FROM shipping_rate_rules rule
JOIN shipping_rates rate
  ON rate.shop_id = rule.shop_id AND rate.carrier_code = rule.carrier_code
 AND rate.service_code = rule.service_code AND rate.zone_code = rule.zone_code;

ALTER TABLE orders
  ADD COLUMN shipping_service_code text,
  ADD COLUMN shipping_service_name text,
  ADD COLUMN shipping_rate_version uuid;