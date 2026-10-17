CREATE TABLE shipping_carriers (
  shop_id uuid NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
  code text NOT NULL CHECK (code ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
  name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 120),
  active boolean NOT NULL DEFAULT false,
  PRIMARY KEY (shop_id, code)
);

CREATE TABLE shipping_zones (
  shop_id uuid NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
  code text NOT NULL CHECK (code ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
  name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 120),
  country_codes jsonb NOT NULL CHECK (
    jsonb_typeof(country_codes) = 'array' AND jsonb_array_length(country_codes) > 0
  ),
  active boolean NOT NULL DEFAULT true,
  PRIMARY KEY (shop_id, code)
);

CREATE TABLE shipping_rates (
  shop_id uuid NOT NULL,
  carrier_code text NOT NULL,
  zone_code text NOT NULL,
  amount numeric(19, 4) NOT NULL CHECK (amount >= 0),
  currency char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
  active boolean NOT NULL DEFAULT true,
  PRIMARY KEY (shop_id, carrier_code, zone_code),
  FOREIGN KEY (shop_id, carrier_code)
    REFERENCES shipping_carriers (shop_id, code) ON DELETE CASCADE,
  FOREIGN KEY (shop_id, zone_code)
    REFERENCES shipping_zones (shop_id, code) ON DELETE CASCADE
);

INSERT INTO shipping_carriers (shop_id, code, name)
SELECT id, carrier.code, carrier.name
FROM shops
CROSS JOIN (VALUES
  ('colissimo', 'Colissimo'),
  ('chronopost', 'Chronopost'),
  ('mondial-relay', 'Mondial Relay'),
  ('la-poste', 'La Poste')
) AS carrier(code, name)
WHERE shops.slug = 'atelier-demo';

INSERT INTO shipping_zones (shop_id, code, name, country_codes)
SELECT id, 'france-ue', 'France et Union européenne',
       '["AT", "BE", "BG", "CY", "CZ", "DE", "DK", "EE", "ES",
         "FI", "FR", "GR", "HR", "HU", "IE", "IT", "LT", "LU",
         "LV", "MT", "NL", "PL", "PT", "RO", "SE", "SI", "SK"]'::jsonb
FROM shops
WHERE slug = 'atelier-demo';
