-- EUROMNI MariaDB 11.8 initial schema, ported from the PostgreSQL migrations.
-- Apply only to a new, empty database. This script creates schema only (no demo data).
-- UUIDs are stored as CHAR(36), JSON as MariaDB JSON, timestamps as DATETIME(6).
SET NAMES utf8mb4 COLLATE utf8mb4_bin;

CREATE TABLE shops (
  id            CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT (UUID()) PRIMARY KEY,
  slug          VARCHAR(512) NOT NULL UNIQUE,
  name          VARCHAR(512) NOT NULL,
  default_locale VARCHAR(512) NOT NULL DEFAULT 'fr-FR',
  currency      char(3) NOT NULL DEFAULT 'EUR',
  timezone      VARCHAR(512) NOT NULL DEFAULT 'Europe/Paris',
  settings      JSON NOT NULL DEFAULT '{}',
  created_at    DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at    DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE shop_locales (
  shop_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  locale     VARCHAR(512) NOT NULL,
  is_default boolean NOT NULL DEFAULT false,
  PRIMARY KEY (shop_id, locale),
  CONSTRAINT fk_shop_locales_shop_id FOREIGN KEY (shop_id) REFERENCES shops(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE users (
  id            CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT (UUID()) PRIMARY KEY,
  email         VARCHAR(320) COLLATE utf8mb4_unicode_ci NOT NULL UNIQUE,
  password_hash VARCHAR(512),
  name          VARCHAR(512) NOT NULL DEFAULT '',
  created_at    DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at    DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE shop_members (
  shop_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  user_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  role       ENUM('owner','admin','editor','support') NOT NULL DEFAULT 'editor',
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (shop_id, user_id),
  CONSTRAINT fk_shop_members_shop_id FOREIGN KEY (shop_id) REFERENCES shops(id) ON DELETE CASCADE,
  CONSTRAINT fk_shop_members_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE customers (
  id            CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT (UUID()) PRIMARY KEY,
  shop_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  email         VARCHAR(320) COLLATE utf8mb4_unicode_ci NOT NULL,
  password_hash VARCHAR(512),
  name          VARCHAR(512) NOT NULL DEFAULT '',
  phone         VARCHAR(512),
  metadata      JSON NOT NULL DEFAULT '{}',
  created_at    DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at    DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE (shop_id, email),
  CONSTRAINT fk_customers_shop_id FOREIGN KEY (shop_id) REFERENCES shops(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE addresses (
  id          CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT (UUID()) PRIMARY KEY,
  shop_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  customer_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin,
  kind        VARCHAR(512) NOT NULL DEFAULT 'shipping',
  company     VARCHAR(512),
  line1       VARCHAR(512) NOT NULL,
  line2       VARCHAR(512),
  city        VARCHAR(512) NOT NULL,
  postal_code VARCHAR(512) NOT NULL,
  region      VARCHAR(512),
  country     char(2) NOT NULL,
  created_at  DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at  DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  CONSTRAINT fk_addresses_shop_id FOREIGN KEY (shop_id) REFERENCES shops(id) ON DELETE CASCADE,
  CONSTRAINT fk_addresses_customer_id FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE INDEX addresses_customer_idx ON addresses (customer_id);

CREATE TABLE customer_consents (
  id          CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT (UUID()) PRIMARY KEY,
  shop_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  customer_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  purpose     VARCHAR(512) NOT NULL,
  granted     boolean NOT NULL,
  granted_at  DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE (customer_id, purpose),
  CONSTRAINT fk_customer_consents_shop_id FOREIGN KEY (shop_id) REFERENCES shops(id) ON DELETE CASCADE,
  CONSTRAINT fk_customer_consents_customer_id FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE sessions (
  id          CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT (UUID()) PRIMARY KEY,
  shop_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  customer_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin,
  user_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin,
  token_hash  BINARY(32) NOT NULL UNIQUE,
  expires_at  DATETIME(6) NOT NULL,
  created_at  DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  CHECK ((customer_id IS NOT NULL) <> (user_id IS NOT NULL)),
  CONSTRAINT fk_sessions_shop_id FOREIGN KEY (shop_id) REFERENCES shops(id) ON DELETE CASCADE,
  CONSTRAINT fk_sessions_customer_id FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE CASCADE,
  CONSTRAINT fk_sessions_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE categories (
  id         CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT (UUID()) PRIMARY KEY,
  shop_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  parent_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin,
  slug       VARCHAR(512) NOT NULL,
  position   integer NOT NULL DEFAULT 0,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE (shop_id, slug),
  CONSTRAINT fk_categories_shop_id FOREIGN KEY (shop_id) REFERENCES shops(id) ON DELETE CASCADE,
  CONSTRAINT fk_categories_parent_id FOREIGN KEY (parent_id) REFERENCES categories(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE category_translations (
  category_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  locale      VARCHAR(512) NOT NULL,
  name        VARCHAR(512) NOT NULL,
  description VARCHAR(512) NOT NULL DEFAULT '',
  PRIMARY KEY (category_id, locale),
  CONSTRAINT fk_category_translations_category_id FOREIGN KEY (category_id) REFERENCES categories(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE products (
  id         CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT (UUID()) PRIMARY KEY,
  shop_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  slug       VARCHAR(512) NOT NULL,
  status     ENUM('draft','active','archived') NOT NULL DEFAULT 'draft',
  metadata   JSON NOT NULL DEFAULT '{}',
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE (shop_id, slug),
  CONSTRAINT fk_products_shop_id FOREIGN KEY (shop_id) REFERENCES shops(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE INDEX products_shop_status_idx ON products (shop_id, status);

CREATE TABLE product_translations (
  product_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  locale      VARCHAR(512) NOT NULL,
  name        VARCHAR(512) NOT NULL,
  description VARCHAR(512) NOT NULL DEFAULT '',
  PRIMARY KEY (product_id, locale),
  CONSTRAINT fk_product_translations_product_id FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE product_categories (
  product_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  category_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  PRIMARY KEY (product_id, category_id),
  CONSTRAINT fk_product_categories_product_id FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE,
  CONSTRAINT fk_product_categories_category_id FOREIGN KEY (category_id) REFERENCES categories(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE media (
  id         CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT (UUID()) PRIMARY KEY,
  shop_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  uri        VARCHAR(512) NOT NULL,
  mime_type  VARCHAR(512) NOT NULL,
  alt        VARCHAR(512) NOT NULL DEFAULT '',
  width      integer,
  height     integer,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  CONSTRAINT fk_media_shop_id FOREIGN KEY (shop_id) REFERENCES shops(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE product_media (
  product_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  media_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  position   integer NOT NULL DEFAULT 0,
  PRIMARY KEY (product_id, media_id),
  CONSTRAINT fk_product_media_product_id FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE,
  CONSTRAINT fk_product_media_media_id FOREIGN KEY (media_id) REFERENCES media(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE variants (
  id         CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT (UUID()) PRIMARY KEY,
  product_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  sku        VARCHAR(512) NOT NULL,
  options    JSON NOT NULL DEFAULT '{}',
  barcode    VARCHAR(512),
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE (product_id, sku),
  CONSTRAINT fk_variants_product_id FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE prices (
  id          CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT (UUID()) PRIMARY KEY,
  variant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  currency    char(3) NOT NULL,
  amount      numeric(19, 4) NOT NULL CHECK (amount >= 0),
  compare_at  numeric(19, 4) CHECK (compare_at IS NULL OR compare_at >= 0),
  UNIQUE (variant_id, currency),
  CONSTRAINT fk_prices_variant_id FOREIGN KEY (variant_id) REFERENCES variants(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE inventory_items (
  variant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
  quantity   integer NOT NULL DEFAULT 0,
  track      boolean NOT NULL DEFAULT true,
  updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  CONSTRAINT fk_inventory_items_variant_id FOREIGN KEY (variant_id) REFERENCES variants(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE carts (
  id          CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT (UUID()) PRIMARY KEY,
  shop_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  customer_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin,
  currency    char(3) NOT NULL,
  created_at  DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at  DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  CONSTRAINT fk_carts_shop_id FOREIGN KEY (shop_id) REFERENCES shops(id) ON DELETE CASCADE,
  CONSTRAINT fk_carts_customer_id FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE INDEX carts_customer_idx ON carts (shop_id, customer_id);

CREATE TABLE cart_lines (
  id         CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT (UUID()) PRIMARY KEY,
  cart_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  variant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL REFERENCES variants(id),
  quantity   integer NOT NULL CHECK (quantity > 0),
  UNIQUE (cart_id, variant_id),
  CONSTRAINT fk_cart_lines_cart_id FOREIGN KEY (cart_id) REFERENCES carts(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE orders (
  id               CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT (UUID()) PRIMARY KEY,
  shop_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  customer_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin,
  number           VARCHAR(512) NOT NULL,
  status           ENUM('pending','awaiting_payment','paid','fulfilling','fulfilled','cancelled','refunded') NOT NULL DEFAULT 'pending',
  currency         char(3) NOT NULL,
  subtotal         numeric(19, 4) NOT NULL DEFAULT 0,
  tax_total        numeric(19, 4) NOT NULL DEFAULT 0,
  shipping_total   numeric(19, 4) NOT NULL DEFAULT 0,
  grand_total      numeric(19, 4) NOT NULL DEFAULT 0,
  shipping_address JSON,
  billing_address  JSON,
  placed_at        DATETIME(6),
  created_at       DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at       DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE (shop_id, number),
  CONSTRAINT fk_orders_shop_id FOREIGN KEY (shop_id) REFERENCES shops(id) ON DELETE RESTRICT,
  CONSTRAINT fk_orders_customer_id FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE INDEX orders_shop_status_idx ON orders (shop_id, status);

CREATE TABLE order_lines (
  id           CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT (UUID()) PRIMARY KEY,
  order_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  variant_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin,
  sku          VARCHAR(512) NOT NULL,
  name         VARCHAR(512) NOT NULL,
  quantity     integer NOT NULL CHECK (quantity > 0),
  unit_price   numeric(19, 4) NOT NULL,
  tax_amount   numeric(19, 4) NOT NULL DEFAULT 0,
  line_total   numeric(19, 4) NOT NULL,
  CONSTRAINT fk_order_lines_order_id FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE CASCADE,
  CONSTRAINT fk_order_lines_variant_id FOREIGN KEY (variant_id) REFERENCES variants(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE payments (
  id           CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT (UUID()) PRIMARY KEY,
  order_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  provider     VARCHAR(512) NOT NULL,
  status       ENUM('pending','authorized','captured','failed','refunded','cancelled') NOT NULL DEFAULT 'pending',
  amount       numeric(19, 4) NOT NULL,
  currency     char(3) NOT NULL,
  provider_ref VARCHAR(512),
  created_at   DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at   DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  CONSTRAINT fk_payments_order_id FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE shipments (
  id           CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT (UUID()) PRIMARY KEY,
  order_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  status       ENUM('pending','ready','shipped','delivered','returned') NOT NULL DEFAULT 'pending',
  carrier      VARCHAR(512),
  tracking     VARCHAR(512),
  shipped_at   DATETIME(6),
  created_at   DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at   DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  CONSTRAINT fk_shipments_order_id FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE pages (
  id         CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT (UUID()) PRIMARY KEY,
  shop_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  slug       VARCHAR(512) NOT NULL,
  status     ENUM('draft','published','archived') NOT NULL DEFAULT 'draft',
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE (shop_id, slug),
  CONSTRAINT fk_pages_shop_id FOREIGN KEY (shop_id) REFERENCES shops(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE page_translations (
  page_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  locale   VARCHAR(512) NOT NULL,
  title    VARCHAR(512) NOT NULL,
  body     JSON NOT NULL DEFAULT '{}',
  PRIMARY KEY (page_id, locale),
  CONSTRAINT fk_page_translations_page_id FOREIGN KEY (page_id) REFERENCES pages(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE menus (
  id      CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT (UUID()) PRIMARY KEY,
  shop_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  handle  VARCHAR(512) NOT NULL,
  UNIQUE (shop_id, handle),
  CONSTRAINT fk_menus_shop_id FOREIGN KEY (shop_id) REFERENCES shops(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE menu_items (
  id        CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT (UUID()) PRIMARY KEY,
  menu_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  parent_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin,
  locale    VARCHAR(512) NOT NULL,
  label     VARCHAR(512) NOT NULL,
  url       VARCHAR(512) NOT NULL,
  position  integer NOT NULL DEFAULT 0,
  CONSTRAINT fk_menu_items_menu_id FOREIGN KEY (menu_id) REFERENCES menus(id) ON DELETE CASCADE,
  CONSTRAINT fk_menu_items_parent_id FOREIGN KEY (parent_id) REFERENCES menu_items(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE modules (
  id          CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT (UUID()) PRIMARY KEY,
  name        VARCHAR(512) NOT NULL UNIQUE,
  abi_version VARCHAR(512) NOT NULL DEFAULT 'euromni.abi.v1',
  wasm_sha256 BINARY(32),
  created_at  DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE shop_modules (
  shop_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  module_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  status       ENUM('registered','installed','disabled','error') NOT NULL DEFAULT 'installed',
  capabilities JSON NOT NULL DEFAULT '{}',
  config       JSON NOT NULL DEFAULT '{}',
  installed_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at   DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (shop_id, module_id),
  CONSTRAINT fk_shop_modules_shop_id FOREIGN KEY (shop_id) REFERENCES shops(id) ON DELETE CASCADE,
  CONSTRAINT fk_shop_modules_module_id FOREIGN KEY (module_id) REFERENCES modules(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE module_kv (
  shop_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  module_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `key` VARCHAR(191) NOT NULL,
  value     JSON NOT NULL,
  PRIMARY KEY (shop_id, module_id, `key`),
  CONSTRAINT fk_module_kv_shop_id FOREIGN KEY (shop_id) REFERENCES shops(id) ON DELETE CASCADE,
  CONSTRAINT fk_module_kv_module_id FOREIGN KEY (module_id) REFERENCES modules(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
CREATE TABLE domain_events (
  id           CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT (UUID()) PRIMARY KEY,
  shop_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  name         VARCHAR(512) NOT NULL,
  payload      JSON NOT NULL,
  published_at DATETIME(6),
  created_at   DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  CONSTRAINT fk_domain_events_shop_id FOREIGN KEY (shop_id) REFERENCES shops(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE INDEX domain_events_unpublished_idx ON domain_events (created_at);

ALTER TABLE carts
  ADD COLUMN access_token_hash BINARY(32),
  ADD COLUMN access_token_expires_at DATETIME(6) NOT NULL DEFAULT (CURRENT_TIMESTAMP(6) + INTERVAL 30 DAY);
CREATE UNIQUE INDEX carts_access_token_hash_idx ON carts (access_token_hash);
ALTER TABLE modules ADD COLUMN build_manifest JSON;
ALTER TABLE orders
  ADD COLUMN shipping_carrier_code VARCHAR(512),
  ADD COLUMN shipping_carrier_name VARCHAR(512),
  ADD COLUMN shipping_zone_code VARCHAR(512),
  ADD COLUMN shipping_zone_name VARCHAR(512);
CREATE TABLE shipping_carriers (
  shop_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  code VARCHAR(64) NOT NULL,
  name VARCHAR(512) NOT NULL,
  active BOOLEAN NOT NULL DEFAULT FALSE,
  PRIMARY KEY (shop_id, code),
  CONSTRAINT fk_shipping_carriers_shop FOREIGN KEY (shop_id) REFERENCES shops(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
CREATE TABLE shipping_zones (
  shop_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  code VARCHAR(64) NOT NULL,
  name VARCHAR(512) NOT NULL,
  country_codes JSON NOT NULL,
  active BOOLEAN NOT NULL DEFAULT TRUE,
  PRIMARY KEY (shop_id, code),
  CONSTRAINT fk_shipping_zones_shop FOREIGN KEY (shop_id) REFERENCES shops(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
CREATE TABLE shipping_rates (
  shop_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  carrier_code VARCHAR(64) NOT NULL,
  zone_code VARCHAR(64) NOT NULL,
  amount DECIMAL(19,4) NOT NULL,
  currency CHAR(3) NOT NULL,
  active BOOLEAN NOT NULL DEFAULT TRUE,
  PRIMARY KEY (shop_id, carrier_code, zone_code),
  CONSTRAINT fk_shipping_rates_carrier FOREIGN KEY (shop_id, carrier_code) REFERENCES shipping_carriers (shop_id, code) ON DELETE CASCADE,
  CONSTRAINT fk_shipping_rates_zone FOREIGN KEY (shop_id, zone_code) REFERENCES shipping_zones (shop_id, code) ON DELETE CASCADE,
  CHECK (amount >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
CREATE TABLE schema_migrations (
  version VARCHAR(191) NOT NULL PRIMARY KEY,
  recorded_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
INSERT INTO schema_migrations (version) VALUES ('0001_mariadb_schema.sql');

CREATE TRIGGER trg_shops_updated_at BEFORE UPDATE ON shops FOR EACH ROW SET NEW.updated_at = CURRENT_TIMESTAMP(6);
CREATE TRIGGER trg_users_updated_at BEFORE UPDATE ON users FOR EACH ROW SET NEW.updated_at = CURRENT_TIMESTAMP(6);
CREATE TRIGGER trg_customers_updated_at BEFORE UPDATE ON customers FOR EACH ROW SET NEW.updated_at = CURRENT_TIMESTAMP(6);
CREATE TRIGGER trg_addresses_updated_at BEFORE UPDATE ON addresses FOR EACH ROW SET NEW.updated_at = CURRENT_TIMESTAMP(6);
CREATE TRIGGER trg_categories_updated_at BEFORE UPDATE ON categories FOR EACH ROW SET NEW.updated_at = CURRENT_TIMESTAMP(6);
CREATE TRIGGER trg_products_updated_at BEFORE UPDATE ON products FOR EACH ROW SET NEW.updated_at = CURRENT_TIMESTAMP(6);
CREATE TRIGGER trg_variants_updated_at BEFORE UPDATE ON variants FOR EACH ROW SET NEW.updated_at = CURRENT_TIMESTAMP(6);
CREATE TRIGGER trg_inventory_items_updated_at BEFORE UPDATE ON inventory_items FOR EACH ROW SET NEW.updated_at = CURRENT_TIMESTAMP(6);
CREATE TRIGGER trg_carts_updated_at BEFORE UPDATE ON carts FOR EACH ROW SET NEW.updated_at = CURRENT_TIMESTAMP(6);
CREATE TRIGGER trg_orders_updated_at BEFORE UPDATE ON orders FOR EACH ROW SET NEW.updated_at = CURRENT_TIMESTAMP(6);
CREATE TRIGGER trg_payments_updated_at BEFORE UPDATE ON payments FOR EACH ROW SET NEW.updated_at = CURRENT_TIMESTAMP(6);
CREATE TRIGGER trg_shipments_updated_at BEFORE UPDATE ON shipments FOR EACH ROW SET NEW.updated_at = CURRENT_TIMESTAMP(6);
CREATE TRIGGER trg_pages_updated_at BEFORE UPDATE ON pages FOR EACH ROW SET NEW.updated_at = CURRENT_TIMESTAMP(6);
CREATE TRIGGER trg_shop_modules_updated_at BEFORE UPDATE ON shop_modules FOR EACH ROW SET NEW.updated_at = CURRENT_TIMESTAMP(6);
