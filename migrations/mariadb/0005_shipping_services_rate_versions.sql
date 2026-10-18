CREATE TABLE shipping_services (
  shop_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  carrier_code VARCHAR(64) NOT NULL,
  code VARCHAR(64) NOT NULL,
  name VARCHAR(120) NOT NULL,
  active BOOLEAN NOT NULL DEFAULT TRUE,
  PRIMARY KEY (shop_id, carrier_code, code),
  CONSTRAINT fk_shipping_services_carrier
    FOREIGN KEY (shop_id, carrier_code)
    REFERENCES shipping_carriers (shop_id, code) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

INSERT INTO shipping_services (shop_id, carrier_code, code, name, active)
SELECT DISTINCT rate.shop_id, rate.carrier_code, 'legacy', 'Offre historique', carrier.active
FROM shipping_rates rate
JOIN shipping_carriers carrier
  ON carrier.shop_id = rate.shop_id AND carrier.code = rate.carrier_code;

ALTER TABLE shipping_rates
  ADD COLUMN service_code VARCHAR(64) NOT NULL DEFAULT 'legacy',
  ADD COLUMN rate_version CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL;

UPDATE shipping_rates SET rate_version = UUID() WHERE rate_version IS NULL;

ALTER TABLE shipping_rates
  MODIFY rate_version CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL;

ALTER TABLE shipping_rate_rules
  ADD COLUMN service_code VARCHAR(64) NOT NULL DEFAULT 'legacy';

ALTER TABLE shipping_rate_rules
  DROP FOREIGN KEY fk_shipping_rate_rules_rate,
  DROP PRIMARY KEY;

ALTER TABLE shipping_rates
  DROP PRIMARY KEY,
  ADD PRIMARY KEY (shop_id, carrier_code, service_code, zone_code),
  ADD CONSTRAINT fk_shipping_rates_service
    FOREIGN KEY (shop_id, carrier_code, service_code)
    REFERENCES shipping_services (shop_id, carrier_code, code) ON DELETE CASCADE;

ALTER TABLE shipping_rate_rules
  ADD PRIMARY KEY (shop_id, carrier_code, service_code, zone_code, min_weight_grams, min_subtotal),
  ADD CONSTRAINT fk_shipping_rate_rules_rate
    FOREIGN KEY (shop_id, carrier_code, service_code, zone_code)
    REFERENCES shipping_rates (shop_id, carrier_code, service_code, zone_code) ON DELETE CASCADE;

CREATE TABLE shipping_rate_versions (
  rate_version CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  shop_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  carrier_code VARCHAR(64) NOT NULL,
  service_code VARCHAR(64) NOT NULL,
  service_name VARCHAR(120) NOT NULL,
  zone_code VARCHAR(64) NOT NULL,
  rate_mode VARCHAR(32) NOT NULL,
  amount numeric(19, 4) NOT NULL CHECK (amount >= 0),
  currency CHAR(3) NOT NULL,
  active BOOLEAN NOT NULL,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (rate_version),
  CHECK (rate_mode IN ('fixed', 'weight_value'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE INDEX shipping_rate_versions_lookup_idx
  ON shipping_rate_versions (shop_id, carrier_code, service_code, zone_code, created_at);

CREATE TABLE shipping_rate_version_rules (
  rate_version CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  min_weight_grams BIGINT NOT NULL CHECK (min_weight_grams >= 0),
  max_weight_grams BIGINT NULL,
  min_subtotal numeric(19, 4) NOT NULL DEFAULT 0 CHECK (min_subtotal >= 0),
  max_subtotal numeric(19, 4) NULL,
  amount numeric(19, 4) NOT NULL CHECK (amount >= 0),
  PRIMARY KEY (rate_version, min_weight_grams, min_subtotal),
  CONSTRAINT fk_shipping_rate_version_rules_version
    FOREIGN KEY (rate_version) REFERENCES shipping_rate_versions (rate_version) ON DELETE CASCADE,
  CHECK (max_weight_grams IS NULL OR max_weight_grams > min_weight_grams),
  CHECK (max_subtotal IS NULL OR max_subtotal > min_subtotal)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

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
  ADD COLUMN shipping_service_code VARCHAR(64) NULL,
  ADD COLUMN shipping_service_name VARCHAR(120) NULL,
  ADD COLUMN shipping_rate_version CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL;