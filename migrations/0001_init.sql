-- EUROMNI — schéma cœur (PostgreSQL 16+)
-- Hébergeur volontairement indifférent : n’importe quelle instance Postgres.
-- Une boutique = une ligne shops. Un déploiement mono-boutique = une seule ligne.

BEGIN;

CREATE EXTENSION IF NOT EXISTS citext;

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$;

-- ---------------------------------------------------------------------------
-- Types
-- ---------------------------------------------------------------------------

CREATE TYPE member_role AS ENUM ('owner', 'admin', 'editor', 'support');
CREATE TYPE product_status AS ENUM ('draft', 'active', 'archived');
CREATE TYPE order_status AS ENUM (
  'pending',
  'awaiting_payment',
  'paid',
  'fulfilling',
  'fulfilled',
  'cancelled',
  'refunded'
);
CREATE TYPE payment_status AS ENUM (
  'pending',
  'authorized',
  'captured',
  'failed',
  'refunded',
  'cancelled'
);
CREATE TYPE shipment_status AS ENUM ('pending', 'ready', 'shipped', 'delivered', 'returned');
CREATE TYPE page_status AS ENUM ('draft', 'published', 'archived');
CREATE TYPE module_status AS ENUM ('registered', 'installed', 'disabled', 'error');

-- ---------------------------------------------------------------------------
-- Tenant / boutique
-- ---------------------------------------------------------------------------

CREATE TABLE shops (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  slug          text NOT NULL UNIQUE,
  name          text NOT NULL,
  default_locale text NOT NULL DEFAULT 'fr-FR',
  currency      char(3) NOT NULL DEFAULT 'EUR',
  timezone      text NOT NULL DEFAULT 'Europe/Paris',
  settings      jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE shop_locales (
  shop_id    uuid NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
  locale     text NOT NULL,
  is_default boolean NOT NULL DEFAULT false,
  PRIMARY KEY (shop_id, locale)
);

CREATE TABLE users (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email         citext NOT NULL UNIQUE,
  password_hash text,
  name          text NOT NULL DEFAULT '',
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE shop_members (
  shop_id    uuid NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
  user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role       member_role NOT NULL DEFAULT 'editor',
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (shop_id, user_id)
);

-- ---------------------------------------------------------------------------
-- Clients boutique (acheteurs, distincts du staff)
-- ---------------------------------------------------------------------------

CREATE TABLE customers (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  shop_id       uuid NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
  email         citext NOT NULL,
  password_hash text,
  name          text NOT NULL DEFAULT '',
  phone         text,
  metadata      jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  UNIQUE (shop_id, email)
);

CREATE TABLE addresses (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  shop_id     uuid NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
  customer_id uuid REFERENCES customers(id) ON DELETE SET NULL,
  kind        text NOT NULL DEFAULT 'shipping', -- shipping | billing | other
  company     text,
  line1       text NOT NULL,
  line2       text,
  city        text NOT NULL,
  postal_code text NOT NULL,
  region      text,
  country     char(2) NOT NULL, -- ISO 3166-1 alpha-2
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX addresses_customer_idx ON addresses (customer_id);

CREATE TABLE customer_consents (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  shop_id     uuid NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
  customer_id uuid NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
  purpose     text NOT NULL, -- marketing | analytics | terms | ...
  granted     boolean NOT NULL,
  granted_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (customer_id, purpose)
);

CREATE TABLE sessions (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  shop_id     uuid NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
  customer_id uuid REFERENCES customers(id) ON DELETE CASCADE,
  user_id     uuid REFERENCES users(id) ON DELETE CASCADE,
  token_hash  bytea NOT NULL UNIQUE,
  expires_at  timestamptz NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  CHECK ((customer_id IS NOT NULL) <> (user_id IS NOT NULL)) -- XOR : client ou staff
);

-- ---------------------------------------------------------------------------
-- Catalogue
-- ---------------------------------------------------------------------------

CREATE TABLE categories (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  shop_id    uuid NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
  parent_id  uuid REFERENCES categories(id) ON DELETE SET NULL,
  slug       text NOT NULL,
  position   integer NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (shop_id, slug)
);

CREATE TABLE category_translations (
  category_id uuid NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
  locale      text NOT NULL,
  name        text NOT NULL,
  description text NOT NULL DEFAULT '',
  PRIMARY KEY (category_id, locale)
);

CREATE TABLE products (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  shop_id    uuid NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
  slug       text NOT NULL,
  status     product_status NOT NULL DEFAULT 'draft',
  metadata   jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (shop_id, slug)
);

CREATE INDEX products_shop_status_idx ON products (shop_id, status);

CREATE TABLE product_translations (
  product_id  uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
  locale      text NOT NULL,
  name        text NOT NULL,
  description text NOT NULL DEFAULT '',
  PRIMARY KEY (product_id, locale)
);

CREATE TABLE product_categories (
  product_id  uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
  category_id uuid NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
  PRIMARY KEY (product_id, category_id)
);

CREATE TABLE media (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  shop_id    uuid NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
  uri        text NOT NULL, -- object storage / CDN, pas le disque du CMS
  mime_type  text NOT NULL,
  alt        text NOT NULL DEFAULT '',
  width      integer,
  height     integer,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE product_media (
  product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
  media_id   uuid NOT NULL REFERENCES media(id) ON DELETE CASCADE,
  position   integer NOT NULL DEFAULT 0,
  PRIMARY KEY (product_id, media_id)
);

CREATE TABLE variants (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
  sku        text NOT NULL,
  options    jsonb NOT NULL DEFAULT '{}'::jsonb, -- {"size":"M","color":"noir"}
  barcode    text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (product_id, sku)
);

CREATE TABLE prices (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  variant_id  uuid NOT NULL REFERENCES variants(id) ON DELETE CASCADE,
  currency    char(3) NOT NULL,
  amount      numeric(19, 4) NOT NULL CHECK (amount >= 0),
  compare_at  numeric(19, 4) CHECK (compare_at IS NULL OR compare_at >= 0),
  UNIQUE (variant_id, currency)
);

CREATE TABLE inventory_items (
  variant_id uuid PRIMARY KEY REFERENCES variants(id) ON DELETE CASCADE,
  quantity   integer NOT NULL DEFAULT 0,
  track      boolean NOT NULL DEFAULT true,
  updated_at timestamptz NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- Panier / commande / paiement / livraison
-- ---------------------------------------------------------------------------

CREATE TABLE carts (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  shop_id     uuid NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
  customer_id uuid REFERENCES customers(id) ON DELETE SET NULL,
  currency    char(3) NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX carts_customer_idx ON carts (shop_id, customer_id);

CREATE TABLE cart_lines (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  cart_id    uuid NOT NULL REFERENCES carts(id) ON DELETE CASCADE,
  variant_id uuid NOT NULL REFERENCES variants(id),
  quantity   integer NOT NULL CHECK (quantity > 0),
  UNIQUE (cart_id, variant_id)
);

CREATE TABLE orders (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  shop_id          uuid NOT NULL REFERENCES shops(id) ON DELETE RESTRICT,
  customer_id      uuid REFERENCES customers(id) ON DELETE SET NULL,
  number           text NOT NULL,
  status           order_status NOT NULL DEFAULT 'pending',
  currency         char(3) NOT NULL,
  subtotal         numeric(19, 4) NOT NULL DEFAULT 0,
  tax_total        numeric(19, 4) NOT NULL DEFAULT 0,
  shipping_total   numeric(19, 4) NOT NULL DEFAULT 0,
  grand_total      numeric(19, 4) NOT NULL DEFAULT 0,
  shipping_address jsonb,
  billing_address  jsonb,
  placed_at        timestamptz,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now(),
  UNIQUE (shop_id, number)
);

CREATE INDEX orders_shop_status_idx ON orders (shop_id, status);

CREATE TABLE order_lines (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id     uuid NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
  variant_id   uuid REFERENCES variants(id) ON DELETE SET NULL,
  sku          text NOT NULL,
  name         text NOT NULL,
  quantity     integer NOT NULL CHECK (quantity > 0),
  unit_price   numeric(19, 4) NOT NULL,
  tax_amount   numeric(19, 4) NOT NULL DEFAULT 0,
  line_total   numeric(19, 4) NOT NULL
);

CREATE TABLE payments (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id     uuid NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
  provider     text NOT NULL,
  status       payment_status NOT NULL DEFAULT 'pending',
  amount       numeric(19, 4) NOT NULL,
  currency     char(3) NOT NULL,
  provider_ref text,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE shipments (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id     uuid NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
  status       shipment_status NOT NULL DEFAULT 'pending',
  carrier      text,
  tracking     text,
  shipped_at   timestamptz,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- CMS
-- ---------------------------------------------------------------------------

CREATE TABLE pages (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  shop_id    uuid NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
  slug       text NOT NULL,
  status     page_status NOT NULL DEFAULT 'draft',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (shop_id, slug)
);

CREATE TABLE page_translations (
  page_id  uuid NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
  locale   text NOT NULL,
  title    text NOT NULL,
  body     jsonb NOT NULL DEFAULT '{}'::jsonb, -- blocs structurés, pas du HTML PHP
  PRIMARY KEY (page_id, locale)
);

CREATE TABLE menus (
  id      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  shop_id uuid NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
  handle  text NOT NULL,
  UNIQUE (shop_id, handle)
);

CREATE TABLE menu_items (
  id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  menu_id   uuid NOT NULL REFERENCES menus(id) ON DELETE CASCADE,
  parent_id uuid REFERENCES menu_items(id) ON DELETE CASCADE,
  locale    text NOT NULL,
  label     text NOT NULL,
  url       text NOT NULL,
  position  integer NOT NULL DEFAULT 0
);

-- ---------------------------------------------------------------------------
-- Modules Wasm (aucun SQL direct depuis Wasm)
-- ---------------------------------------------------------------------------

CREATE TABLE modules (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name        text NOT NULL UNIQUE,
  abi_version text NOT NULL DEFAULT 'euromni.abi.v1',
  wasm_sha256 bytea,
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE shop_modules (
  shop_id      uuid NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
  module_id    uuid NOT NULL REFERENCES modules(id) ON DELETE CASCADE,
  status       module_status NOT NULL DEFAULT 'installed',
  capabilities text[] NOT NULL DEFAULT '{}',
  config       jsonb NOT NULL DEFAULT '{}'::jsonb,
  installed_at timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (shop_id, module_id)
);

CREATE TABLE module_kv (
  shop_id   uuid NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
  module_id uuid NOT NULL REFERENCES modules(id) ON DELETE CASCADE,
  key       text NOT NULL,
  value     jsonb NOT NULL,
  PRIMARY KEY (shop_id, module_id, key)
);

-- Outbox d’événements métier (hooks Wasm + webhooks HTTP)
CREATE TABLE domain_events (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  shop_id      uuid NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
  name         text NOT NULL, -- product.updated, order.paid, ...
  payload      jsonb NOT NULL,
  published_at timestamptz,
  created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX domain_events_unpublished_idx
  ON domain_events (created_at)
  WHERE published_at IS NULL;

-- ---------------------------------------------------------------------------
-- Triggers updated_at
-- ---------------------------------------------------------------------------

DO $$
DECLARE
  t text;
BEGIN
  FOREACH t IN ARRAY ARRAY[
    'shops', 'users', 'customers', 'addresses', 'categories', 'products',
    'variants', 'inventory_items', 'carts', 'orders', 'payments',
    'shipments', 'pages', 'shop_modules'
  ]
  LOOP
    EXECUTE format(
      'CREATE TRIGGER trg_%I_updated_at BEFORE UPDATE ON %I
       FOR EACH ROW EXECUTE FUNCTION set_updated_at()',
      t, t
    );
  END LOOP;
END;
$$;

COMMIT;
