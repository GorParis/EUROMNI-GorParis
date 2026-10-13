-- Second active catalog product so listing is not a singleton seed.
BEGIN;

INSERT INTO products (id, shop_id, slug, status, metadata)
VALUES (
  'e0e00000-0001-4000-8000-000000000044',
  'e0e00000-0001-4000-8000-000000000001',
  'ceinture-cuir',
  'active',
  '{"origin":"FR"}'::jsonb
);

INSERT INTO product_translations (product_id, locale, name, description) VALUES
  ('e0e00000-0001-4000-8000-000000000044', 'fr-FR', 'Ceinture en cuir', 'Cuir tanné végétal, boucle laiton.'),
  ('e0e00000-0001-4000-8000-000000000044', 'en-GB', 'Leather belt', 'Vegetable-tanned leather, brass buckle.');

INSERT INTO product_categories (product_id, category_id)
VALUES (
  'e0e00000-0001-4000-8000-000000000044',
  'e0e00000-0001-4000-8000-000000000030'
);

INSERT INTO media (id, shop_id, uri, mime_type, alt, width, height)
VALUES (
  'e0e00000-0001-4000-8000-000000000045',
  'e0e00000-0001-4000-8000-000000000001',
  'https://cdn.example/atelier-demo/ceinture-cuir.jpg',
  'image/jpeg',
  'Ceinture en cuir brun',
  1200,
  1200
);

INSERT INTO product_media (product_id, media_id, position)
VALUES (
  'e0e00000-0001-4000-8000-000000000044',
  'e0e00000-0001-4000-8000-000000000045',
  0
);

INSERT INTO variants (id, product_id, sku, options, barcode)
VALUES (
  'e0e00000-0001-4000-8000-000000000046',
  'e0e00000-0001-4000-8000-000000000044',
  'CEINT-CUIR-BRN',
  '{"color":"brun","size":"85"}'::jsonb,
  '3700000000002'
);

INSERT INTO prices (id, variant_id, currency, amount, compare_at)
VALUES (
  'e0e00000-0001-4000-8000-000000000047',
  'e0e00000-0001-4000-8000-000000000046',
  'EUR',
  45.0000,
  NULL
);

INSERT INTO inventory_items (variant_id, quantity, track)
VALUES (
  'e0e00000-0001-4000-8000-000000000046',
  12,
  true
);

INSERT INTO schema_migrations (version) VALUES ('0005_seed_second_product.sql');

COMMIT;
