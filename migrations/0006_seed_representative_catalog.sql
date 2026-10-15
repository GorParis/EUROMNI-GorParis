-- Representative products for the storefront: single-SKU goods and products
-- with selectable size variants.

UPDATE media
SET uri = '/storefront/images/cabas-lin.svg', mime_type = 'image/svg+xml'
WHERE id = 'e0e00000-0001-4000-8000-000000000041';

UPDATE media
SET uri = '/storefront/images/ceinture-cuir.svg', mime_type = 'image/svg+xml'
WHERE id = 'e0e00000-0001-4000-8000-000000000045';

INSERT INTO variants (id, product_id, sku, options, barcode)
VALUES
  ('e0e00000-0001-4000-8000-000000000048', 'e0e00000-0001-4000-8000-000000000044', 'CEINT-CUIR-BRN-90', '{"color":"brun","size":"90"}'::jsonb, '3700000000003'),
  ('e0e00000-0001-4000-8000-000000000049', 'e0e00000-0001-4000-8000-000000000044', 'CEINT-CUIR-BRN-95', '{"color":"brun","size":"95"}'::jsonb, '3700000000004');

INSERT INTO prices (id, variant_id, currency, amount, compare_at)
VALUES
  ('e0e00000-0001-4000-8000-000000000058', 'e0e00000-0001-4000-8000-000000000048', 'EUR', 45.0000, NULL),
  ('e0e00000-0001-4000-8000-000000000059', 'e0e00000-0001-4000-8000-000000000049', 'EUR', 47.0000, NULL);

INSERT INTO inventory_items (variant_id, quantity, track)
VALUES
  ('e0e00000-0001-4000-8000-000000000048', 9, true),
  ('e0e00000-0001-4000-8000-000000000049', 6, true);

INSERT INTO products (id, shop_id, slug, status, metadata)
VALUES
  ('e0e00000-0001-4000-8000-000000000050', 'e0e00000-0001-4000-8000-000000000001', 'chemise-lin', 'active', '{"origin":"FR","material":"lin"}'::jsonb),
  ('e0e00000-0001-4000-8000-000000000054', 'e0e00000-0001-4000-8000-000000000001', 'tasse-gres', 'active', '{"origin":"FR","material":"gres"}'::jsonb),
  ('e0e00000-0001-4000-8000-000000000056', 'e0e00000-0001-4000-8000-000000000001', 'bougie-cedre', 'active', '{"origin":"FR","material":"cire-vegetale"}'::jsonb);

INSERT INTO product_translations (product_id, locale, name, description)
VALUES
  ('e0e00000-0001-4000-8000-000000000050', 'fr-FR', 'Chemise en lin', 'Une chemise ample en lin européen, légère et respirante.'),
  ('e0e00000-0001-4000-8000-000000000050', 'en-GB', 'Linen shirt', 'A relaxed-fit shirt in breathable European linen.'),
  ('e0e00000-0001-4000-8000-000000000054', 'fr-FR', 'Tasse en grès', 'Une tasse en grès façonnée et émaillée à la main.'),
  ('e0e00000-0001-4000-8000-000000000054', 'en-GB', 'Stoneware cup', 'A hand-finished stoneware cup, made in small batches.'),
  ('e0e00000-0001-4000-8000-000000000056', 'fr-FR', 'Bougie au cèdre', 'Une bougie en cire végétale parfumée au cèdre et aux herbes sèches.'),
  ('e0e00000-0001-4000-8000-000000000056', 'en-GB', 'Cedar candle', 'A plant-wax candle with cedar and dried herbs.');

INSERT INTO variants (id, product_id, sku, options, barcode)
VALUES
  ('e0e00000-0001-4000-8000-000000000051', 'e0e00000-0001-4000-8000-000000000050', 'CHEMISE-LIN-S', '{"taille":"S"}'::jsonb, '3700000000005'),
  ('e0e00000-0001-4000-8000-000000000052', 'e0e00000-0001-4000-8000-000000000050', 'CHEMISE-LIN-M', '{"taille":"M"}'::jsonb, '3700000000006'),
  ('e0e00000-0001-4000-8000-000000000053', 'e0e00000-0001-4000-8000-000000000050', 'CHEMISE-LIN-L', '{"taille":"L"}'::jsonb, '3700000000007'),
  ('e0e00000-0001-4000-8000-000000000055', 'e0e00000-0001-4000-8000-000000000054', 'TASSE-GRES-SABLE', '{}'::jsonb, '3700000000008'),
  ('e0e00000-0001-4000-8000-000000000057', 'e0e00000-0001-4000-8000-000000000056', 'BOUGIE-CEDRE-180', '{}'::jsonb, '3700000000009');

INSERT INTO prices (id, variant_id, currency, amount, compare_at)
VALUES
  ('e0e00000-0001-4000-8000-000000000060', 'e0e00000-0001-4000-8000-000000000051', 'EUR', 79.0000, NULL),
  ('e0e00000-0001-4000-8000-000000000061', 'e0e00000-0001-4000-8000-000000000052', 'EUR', 79.0000, NULL),
  ('e0e00000-0001-4000-8000-000000000062', 'e0e00000-0001-4000-8000-000000000053', 'EUR', 82.0000, NULL),
  ('e0e00000-0001-4000-8000-000000000063', 'e0e00000-0001-4000-8000-000000000055', 'EUR', 28.0000, NULL),
  ('e0e00000-0001-4000-8000-000000000064', 'e0e00000-0001-4000-8000-000000000057', 'EUR', 24.0000, NULL);

INSERT INTO inventory_items (variant_id, quantity, track)
VALUES
  ('e0e00000-0001-4000-8000-000000000051', 5, true),
  ('e0e00000-0001-4000-8000-000000000052', 8, true),
  ('e0e00000-0001-4000-8000-000000000053', 4, true),
  ('e0e00000-0001-4000-8000-000000000055', 14, true),
  ('e0e00000-0001-4000-8000-000000000057', 18, true);

INSERT INTO media (id, shop_id, uri, mime_type, alt, width, height)
VALUES
  ('e0e00000-0001-4000-8000-000000000065', 'e0e00000-0001-4000-8000-000000000001', '/storefront/images/chemise-lin.svg', 'image/svg+xml', 'Chemise en lin écru', 900, 700),
  ('e0e00000-0001-4000-8000-000000000066', 'e0e00000-0001-4000-8000-000000000001', '/storefront/images/tasse-gres.svg', 'image/svg+xml', 'Tasse en grès émaillée', 900, 700),
  ('e0e00000-0001-4000-8000-000000000067', 'e0e00000-0001-4000-8000-000000000001', '/storefront/images/bougie-cedre.svg', 'image/svg+xml', 'Bougie artisanale au cèdre', 900, 700);

INSERT INTO product_media (product_id, media_id, position)
VALUES
  ('e0e00000-0001-4000-8000-000000000050', 'e0e00000-0001-4000-8000-000000000065', 0),
  ('e0e00000-0001-4000-8000-000000000054', 'e0e00000-0001-4000-8000-000000000066', 0),
  ('e0e00000-0001-4000-8000-000000000056', 'e0e00000-0001-4000-8000-000000000067', 0);
