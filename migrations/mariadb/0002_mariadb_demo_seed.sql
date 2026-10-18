INSERT INTO shops (id, slug, name, default_locale, currency, timezone, settings)
VALUES (
  'e0e00000-0001-4000-8000-000000000001',
  'atelier-demo',
  'Atelier Demo',
  'fr-FR',
  'EUR',
  'Europe/Paris',
  '{"theme":"atelier","vat_included":true}'
);

INSERT INTO shop_locales (shop_id, locale, is_default)
VALUES
  ('e0e00000-0001-4000-8000-000000000001', 'fr-FR', true),
  ('e0e00000-0001-4000-8000-000000000001', 'en-GB', false);

INSERT INTO users (id, email, password_hash, name)
VALUES (
  'e0e00000-0001-4000-8000-000000000010',
  'owner@atelier-demo.example',
  NULL,
  'Camille Martin'
);

INSERT INTO shop_members (shop_id, user_id, role)
VALUES ('e0e00000-0001-4000-8000-000000000001', 'e0e00000-0001-4000-8000-000000000010', 'owner');

INSERT INTO categories (id, shop_id, slug, position)
VALUES ('e0e00000-0001-4000-8000-000000000030', 'e0e00000-0001-4000-8000-000000000001', 'sacs', 1);

INSERT INTO category_translations (category_id, locale, name, description)
VALUES
  ('e0e00000-0001-4000-8000-000000000030', 'fr-FR', 'Sacs', 'Sacs en toile et cuir'),
  ('e0e00000-0001-4000-8000-000000000030', 'en-GB', 'Bags', 'Canvas and leather bags');

INSERT INTO products (id, shop_id, slug, status, metadata)
VALUES
  ('e0e00000-0001-4000-8000-000000000040', 'e0e00000-0001-4000-8000-000000000001', 'cabas-lin', 'active', '{"origin":"FR","material":"lin"}'),
  ('e0e00000-0001-4000-8000-000000000044', 'e0e00000-0001-4000-8000-000000000001', 'ceinture-cuir', 'active', '{"origin":"FR","material":"cuir"}'),
  ('e0e00000-0001-4000-8000-000000000050', 'e0e00000-0001-4000-8000-000000000001', 'chemise-lin', 'active', '{"origin":"FR","material":"lin"}'),
  ('e0e00000-0001-4000-8000-000000000054', 'e0e00000-0001-4000-8000-000000000001', 'tasse-gres', 'active', '{"origin":"FR","material":"gres"}'),
  ('e0e00000-0001-4000-8000-000000000056', 'e0e00000-0001-4000-8000-000000000001', 'bougie-cedre', 'active', '{"origin":"FR","material":"cire-vegetale"}');

INSERT INTO product_translations (product_id, locale, name, description)
VALUES
  ('e0e00000-0001-4000-8000-000000000040', 'fr-FR', 'Cabas en lin', 'Cabas quotidien, anse cuir.'),
  ('e0e00000-0001-4000-8000-000000000040', 'en-GB', 'Linen tote', 'Everyday tote with leather handles.'),
  ('e0e00000-0001-4000-8000-000000000044', 'fr-FR', 'Ceinture en cuir', 'Cuir tanné végétal, boucle laiton.'),
  ('e0e00000-0001-4000-8000-000000000044', 'en-GB', 'Leather belt', 'Vegetable-tanned leather, brass buckle.'),
  ('e0e00000-0001-4000-8000-000000000050', 'fr-FR', 'Chemise en lin', 'Une chemise ample en lin européen.'),
  ('e0e00000-0001-4000-8000-000000000050', 'en-GB', 'Linen shirt', 'A relaxed-fit shirt in European linen.'),
  ('e0e00000-0001-4000-8000-000000000054', 'fr-FR', 'Tasse en grès', 'Une tasse en grès façonnée à la main.'),
  ('e0e00000-0001-4000-8000-000000000054', 'en-GB', 'Stoneware cup', 'A hand-finished stoneware cup.'),
  ('e0e00000-0001-4000-8000-000000000056', 'fr-FR', 'Bougie au cèdre', 'Une bougie en cire végétale parfumée au cèdre.'),
  ('e0e00000-0001-4000-8000-000000000056', 'en-GB', 'Cedar candle', 'A plant-wax candle scented with cedar.');

INSERT INTO product_categories (product_id, category_id)
VALUES
  ('e0e00000-0001-4000-8000-000000000040', 'e0e00000-0001-4000-8000-000000000030'),
  ('e0e00000-0001-4000-8000-000000000044', 'e0e00000-0001-4000-8000-000000000030'),
  ('e0e00000-0001-4000-8000-000000000050', 'e0e00000-0001-4000-8000-000000000030'),
  ('e0e00000-0001-4000-8000-000000000054', 'e0e00000-0001-4000-8000-000000000030'),
  ('e0e00000-0001-4000-8000-000000000056', 'e0e00000-0001-4000-8000-000000000030');

INSERT INTO media (id, shop_id, uri, mime_type, alt, width, height)
VALUES
  ('e0e00000-0001-4000-8000-000000000041', 'e0e00000-0001-4000-8000-000000000001', '/storefront/images/cabas-lin.svg', 'image/svg+xml', 'Cabas en lin naturel', 900, 700),
  ('e0e00000-0001-4000-8000-000000000045', 'e0e00000-0001-4000-8000-000000000001', '/storefront/images/ceinture-cuir.svg', 'image/svg+xml', 'Ceinture en cuir brun', 900, 700),
  ('e0e00000-0001-4000-8000-000000000065', 'e0e00000-0001-4000-8000-000000000001', '/storefront/images/chemise-lin.svg', 'image/svg+xml', 'Chemise en lin écru', 900, 700),
  ('e0e00000-0001-4000-8000-000000000066', 'e0e00000-0001-4000-8000-000000000001', '/storefront/images/tasse-gres.svg', 'image/svg+xml', 'Tasse en grès émaillée', 900, 700),
  ('e0e00000-0001-4000-8000-000000000067', 'e0e00000-0001-4000-8000-000000000001', '/storefront/images/bougie-cedre.svg', 'image/svg+xml', 'Bougie artisanale au cèdre', 900, 700);

INSERT INTO product_media (product_id, media_id, position)
VALUES
  ('e0e00000-0001-4000-8000-000000000040', 'e0e00000-0001-4000-8000-000000000041', 0),
  ('e0e00000-0001-4000-8000-000000000044', 'e0e00000-0001-4000-8000-000000000045', 0),
  ('e0e00000-0001-4000-8000-000000000050', 'e0e00000-0001-4000-8000-000000000065', 0),
  ('e0e00000-0001-4000-8000-000000000054', 'e0e00000-0001-4000-8000-000000000066', 0),
  ('e0e00000-0001-4000-8000-000000000056', 'e0e00000-0001-4000-8000-000000000067', 0);

INSERT INTO variants (id, product_id, sku, options, barcode)
VALUES
  ('e0e00000-0001-4000-8000-000000000042', 'e0e00000-0001-4000-8000-000000000040', 'CABAS-LIN-NAT', '{"color":"naturel","size":"unique"}', '3700000000001'),
  ('e0e00000-0001-4000-8000-000000000046', 'e0e00000-0001-4000-8000-000000000044', 'CEINT-CUIR-BRN', '{"color":"brun","size":"85"}', '3700000000002'),
  ('e0e00000-0001-4000-8000-000000000048', 'e0e00000-0001-4000-8000-000000000044', 'CEINT-CUIR-BRN-90', '{"color":"brun","size":"90"}', '3700000000003'),
  ('e0e00000-0001-4000-8000-000000000049', 'e0e00000-0001-4000-8000-000000000044', 'CEINT-CUIR-BRN-95', '{"color":"brun","size":"95"}', '3700000000004'),
  ('e0e00000-0001-4000-8000-000000000051', 'e0e00000-0001-4000-8000-000000000050', 'CHEMISE-LIN-S', '{"taille":"S"}', '3700000000005'),
  ('e0e00000-0001-4000-8000-000000000052', 'e0e00000-0001-4000-8000-000000000050', 'CHEMISE-LIN-M', '{"taille":"M"}', '3700000000006'),
  ('e0e00000-0001-4000-8000-000000000053', 'e0e00000-0001-4000-8000-000000000050', 'CHEMISE-LIN-L', '{"taille":"L"}', '3700000000007'),
  ('e0e00000-0001-4000-8000-000000000055', 'e0e00000-0001-4000-8000-000000000054', 'TASSE-GRES-SABLE', '{}', '3700000000008'),
  ('e0e00000-0001-4000-8000-000000000057', 'e0e00000-0001-4000-8000-000000000056', 'BOUGIE-CEDRE-180', '{}', '3700000000009');

INSERT INTO prices (id, variant_id, currency, amount, compare_at)
VALUES
  ('e0e00000-0001-4000-8000-000000000043', 'e0e00000-0001-4000-8000-000000000042', 'EUR', 89.0000, 110.0000),
  ('e0e00000-0001-4000-8000-000000000047', 'e0e00000-0001-4000-8000-000000000046', 'EUR', 45.0000, NULL),
  ('e0e00000-0001-4000-8000-000000000058', 'e0e00000-0001-4000-8000-000000000048', 'EUR', 45.0000, NULL),
  ('e0e00000-0001-4000-8000-000000000059', 'e0e00000-0001-4000-8000-000000000049', 'EUR', 47.0000, NULL),
  ('e0e00000-0001-4000-8000-000000000060', 'e0e00000-0001-4000-8000-000000000051', 'EUR', 79.0000, NULL),
  ('e0e00000-0001-4000-8000-000000000061', 'e0e00000-0001-4000-8000-000000000052', 'EUR', 79.0000, NULL),
  ('e0e00000-0001-4000-8000-000000000062', 'e0e00000-0001-4000-8000-000000000053', 'EUR', 82.0000, NULL),
  ('e0e00000-0001-4000-8000-000000000063', 'e0e00000-0001-4000-8000-000000000055', 'EUR', 28.0000, NULL),
  ('e0e00000-0001-4000-8000-000000000064', 'e0e00000-0001-4000-8000-000000000057', 'EUR', 24.0000, NULL);

INSERT INTO inventory_items (variant_id, quantity, track)
VALUES
  ('e0e00000-0001-4000-8000-000000000042', 24, true),
  ('e0e00000-0001-4000-8000-000000000046', 12, true),
  ('e0e00000-0001-4000-8000-000000000048', 9, true),
  ('e0e00000-0001-4000-8000-000000000049', 6, true),
  ('e0e00000-0001-4000-8000-000000000051', 5, true),
  ('e0e00000-0001-4000-8000-000000000052', 8, true),
  ('e0e00000-0001-4000-8000-000000000053', 4, true),
  ('e0e00000-0001-4000-8000-000000000055', 14, true),
  ('e0e00000-0001-4000-8000-000000000057', 18, true);

INSERT INTO shipping_carriers (shop_id, code, name, active)
VALUES ('e0e00000-0001-4000-8000-000000000001', 'colissimo', 'Colissimo', true);

INSERT INTO shipping_zones (shop_id, code, name, country_codes, active)
VALUES (
  'e0e00000-0001-4000-8000-000000000001',
  'france-ue',
  'France et Union européenne',
  '["AT","BE","BG","CY","CZ","DE","DK","EE","ES","FI","FR","GR","HR","HU","IE","IT","LT","LU","LV","MT","NL","PL","PT","RO","SE","SI","SK"]',
  true
);

INSERT INTO shipping_rates (shop_id, carrier_code, zone_code, amount, currency, active)
VALUES ('e0e00000-0001-4000-8000-000000000001', 'colissimo', 'france-ue', 6.9000, 'EUR', true);

INSERT INTO modules (id, name, abi_version)
VALUES ('e0e00000-0001-4000-8000-000000000080', 'demo.loyalty', 'euromni.abi.v1');

INSERT INTO shop_modules (shop_id, module_id, status, capabilities, config)
VALUES (
  'e0e00000-0001-4000-8000-000000000001',
  'e0e00000-0001-4000-8000-000000000080',
  'installed',
  '["catalog","customer","hooks","module.kv"]',
  '{"points_per_euro":1}'
);
