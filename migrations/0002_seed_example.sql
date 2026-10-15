-- Jeu exemple EUROMNI (boutique démo). À appliquer après 0001_init.sql.
-- IDs fixes pour coller au mapping ABI (docs/abi.md).

INSERT INTO shops (id, slug, name, default_locale, currency, timezone, settings)
VALUES (
  'e0e00000-0001-4000-8000-000000000001',
  'atelier-demo',
  'Atelier Demo',
  'fr-FR',
  'EUR',
  'Europe/Paris',
  '{"theme":"atelier","vat_included":true}'::jsonb
);

INSERT INTO shop_locales (shop_id, locale, is_default) VALUES
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
VALUES (
  'e0e00000-0001-4000-8000-000000000001',
  'e0e00000-0001-4000-8000-000000000010',
  'owner'
);

INSERT INTO customers (id, shop_id, email, password_hash, name, phone, metadata)
VALUES (
  'e0e00000-0001-4000-8000-000000000020',
  'e0e00000-0001-4000-8000-000000000001',
  'lea@client.example',
  NULL,
  'Léa Dupont',
  '+33601020304',
  '{"newsletter":true}'::jsonb
);

INSERT INTO addresses (id, shop_id, customer_id, kind, line1, city, postal_code, country)
VALUES (
  'e0e00000-0001-4000-8000-000000000021',
  'e0e00000-0001-4000-8000-000000000001',
  'e0e00000-0001-4000-8000-000000000020',
  'shipping',
  '12 rue des Ateliers',
  'Lyon',
  '69001',
  'FR'
);

INSERT INTO customer_consents (id, shop_id, customer_id, purpose, granted)
VALUES (
  'e0e00000-0001-4000-8000-000000000022',
  'e0e00000-0001-4000-8000-000000000001',
  'e0e00000-0001-4000-8000-000000000020',
  'terms',
  true
);

INSERT INTO sessions (id, shop_id, customer_id, token_hash, expires_at)
VALUES (
  'e0e00000-0001-4000-8000-000000000023',
  'e0e00000-0001-4000-8000-000000000001',
  'e0e00000-0001-4000-8000-000000000020',
  decode('cafecafecafecafecafecafecafecafe', 'hex'),
  now() + interval '7 days'
);

INSERT INTO categories (id, shop_id, slug, position)
VALUES (
  'e0e00000-0001-4000-8000-000000000030',
  'e0e00000-0001-4000-8000-000000000001',
  'sacs',
  1
);

INSERT INTO category_translations (category_id, locale, name, description) VALUES
  ('e0e00000-0001-4000-8000-000000000030', 'fr-FR', 'Sacs', 'Sacs en toile et cuir'),
  ('e0e00000-0001-4000-8000-000000000030', 'en-GB', 'Bags', 'Canvas and leather bags');

INSERT INTO products (id, shop_id, slug, status, metadata)
VALUES (
  'e0e00000-0001-4000-8000-000000000040',
  'e0e00000-0001-4000-8000-000000000001',
  'cabas-lin',
  'active',
  '{"origin":"FR"}'::jsonb
);

INSERT INTO product_translations (product_id, locale, name, description) VALUES
  ('e0e00000-0001-4000-8000-000000000040', 'fr-FR', 'Cabas en lin', 'Cabas quotidien, anse cuir.'),
  ('e0e00000-0001-4000-8000-000000000040', 'en-GB', 'Linen tote', 'Everyday tote, leather handles.');

INSERT INTO product_categories (product_id, category_id)
VALUES (
  'e0e00000-0001-4000-8000-000000000040',
  'e0e00000-0001-4000-8000-000000000030'
);

INSERT INTO media (id, shop_id, uri, mime_type, alt, width, height)
VALUES (
  'e0e00000-0001-4000-8000-000000000041',
  'e0e00000-0001-4000-8000-000000000001',
  'https://cdn.example/atelier-demo/cabas-lin.jpg',
  'image/jpeg',
  'Cabas en lin naturel',
  1200,
  1200
);

INSERT INTO product_media (product_id, media_id, position)
VALUES (
  'e0e00000-0001-4000-8000-000000000040',
  'e0e00000-0001-4000-8000-000000000041',
  0
);

INSERT INTO variants (id, product_id, sku, options, barcode)
VALUES (
  'e0e00000-0001-4000-8000-000000000042',
  'e0e00000-0001-4000-8000-000000000040',
  'CABAS-LIN-NAT',
  '{"color":"naturel","size":"unique"}'::jsonb,
  '3700000000001'
);

INSERT INTO prices (id, variant_id, currency, amount, compare_at)
VALUES (
  'e0e00000-0001-4000-8000-000000000043',
  'e0e00000-0001-4000-8000-000000000042',
  'EUR',
  89.0000,
  110.0000
);

INSERT INTO inventory_items (variant_id, quantity, track)
VALUES (
  'e0e00000-0001-4000-8000-000000000042',
  24,
  true
);

INSERT INTO carts (id, shop_id, customer_id, currency)
VALUES (
  'e0e00000-0001-4000-8000-000000000050',
  'e0e00000-0001-4000-8000-000000000001',
  'e0e00000-0001-4000-8000-000000000020',
  'EUR'
);

INSERT INTO cart_lines (id, cart_id, variant_id, quantity)
VALUES (
  'e0e00000-0001-4000-8000-000000000051',
  'e0e00000-0001-4000-8000-000000000050',
  'e0e00000-0001-4000-8000-000000000042',
  1
);

INSERT INTO orders (
  id, shop_id, customer_id, number, status, currency,
  subtotal, tax_total, shipping_total, grand_total,
  shipping_address, billing_address, placed_at
) VALUES (
  'e0e00000-0001-4000-8000-000000000060',
  'e0e00000-0001-4000-8000-000000000001',
  'e0e00000-0001-4000-8000-000000000020',
  'AD-1001',
  'paid',
  'EUR',
  89.0000,
  0.0000,
  6.9000,
  95.9000,
  '{"line1":"12 rue des Ateliers","city":"Lyon","postal_code":"69001","country":"FR"}'::jsonb,
  '{"line1":"12 rue des Ateliers","city":"Lyon","postal_code":"69001","country":"FR"}'::jsonb,
  now() - interval '2 days'
);

INSERT INTO order_lines (id, order_id, variant_id, sku, name, quantity, unit_price, tax_amount, line_total)
VALUES (
  'e0e00000-0001-4000-8000-000000000061',
  'e0e00000-0001-4000-8000-000000000060',
  'e0e00000-0001-4000-8000-000000000042',
  'CABAS-LIN-NAT',
  'Cabas en lin',
  1,
  89.0000,
  0.0000,
  89.0000
);

INSERT INTO payments (id, order_id, provider, status, amount, currency, provider_ref)
VALUES (
  'e0e00000-0001-4000-8000-000000000062',
  'e0e00000-0001-4000-8000-000000000060',
  'demo-psp',
  'captured',
  95.9000,
  'EUR',
  'pay_demo_1001'
);

INSERT INTO shipments (id, order_id, status, carrier, tracking, shipped_at)
VALUES (
  'e0e00000-0001-4000-8000-000000000063',
  'e0e00000-0001-4000-8000-000000000060',
  'shipped',
  'Colissimo',
  '6CDEMO0000001',
  now() - interval '1 day'
);

INSERT INTO pages (id, shop_id, slug, status)
VALUES (
  'e0e00000-0001-4000-8000-000000000070',
  'e0e00000-0001-4000-8000-000000000001',
  'a-propos',
  'published'
);

INSERT INTO page_translations (page_id, locale, title, body) VALUES
  (
    'e0e00000-0001-4000-8000-000000000070',
    'fr-FR',
    'À propos',
    '{"blocks":[{"type":"richtext","text":"Atelier Demo — pièces cousues à Lyon."}]}'::jsonb
  );

INSERT INTO menus (id, shop_id, handle)
VALUES (
  'e0e00000-0001-4000-8000-000000000071',
  'e0e00000-0001-4000-8000-000000000001',
  'header'
);

INSERT INTO menu_items (id, menu_id, locale, label, url, position)
VALUES (
  'e0e00000-0001-4000-8000-000000000072',
  'e0e00000-0001-4000-8000-000000000071',
  'fr-FR',
  'Sacs',
  '/c/sacs',
  0
);

INSERT INTO modules (id, name, abi_version)
VALUES (
  'e0e00000-0001-4000-8000-000000000080',
  'demo.loyalty',
  'euromni.abi.v1'
);

INSERT INTO shop_modules (shop_id, module_id, status, capabilities, config)
VALUES (
  'e0e00000-0001-4000-8000-000000000001',
  'e0e00000-0001-4000-8000-000000000080',
  'installed',
  ARRAY['catalog','customer','hooks','module.kv']::text[],
  '{"points_per_euro":1}'::jsonb
);

INSERT INTO module_kv (shop_id, module_id, key, value)
VALUES (
  'e0e00000-0001-4000-8000-000000000001',
  'e0e00000-0001-4000-8000-000000000080',
  'points:e0e00000-0001-4000-8000-000000000020',
  '{"balance":96}'::jsonb
);

INSERT INTO domain_events (id, shop_id, name, payload, published_at)
VALUES (
  'e0e00000-0001-4000-8000-000000000090',
  'e0e00000-0001-4000-8000-000000000001',
  'order.paid',
  '{"order_id":"e0e00000-0001-4000-8000-000000000060","number":"AD-1001"}'::jsonb,
  now() - interval '2 days'
);
