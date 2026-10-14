PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS shops (
    id TEXT PRIMARY KEY,
    default_locale TEXT NOT NULL,
    currency TEXT NOT NULL CHECK (length(currency) = 3)
);

CREATE TABLE IF NOT EXISTS categories (
    id TEXT PRIMARY KEY,
    shop_id TEXT NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    slug TEXT NOT NULL,
    position INTEGER NOT NULL DEFAULT 0,
    UNIQUE (shop_id, slug)
);

CREATE TABLE IF NOT EXISTS category_translations (
    category_id TEXT NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    locale TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (category_id, locale)
);

CREATE TABLE IF NOT EXISTS products (
    id TEXT PRIMARY KEY,
    shop_id TEXT NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    slug TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('draft', 'active', 'archived')),
    UNIQUE (shop_id, slug)
);

CREATE INDEX IF NOT EXISTS products_shop_status_idx ON products(shop_id, status);

CREATE TABLE IF NOT EXISTS product_translations (
    product_id TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    locale TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (product_id, locale)
);

CREATE TABLE IF NOT EXISTS product_categories (
    product_id TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    category_id TEXT NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    PRIMARY KEY (product_id, category_id)
);

CREATE TABLE IF NOT EXISTS media (
    id TEXT PRIMARY KEY,
    shop_id TEXT NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    uri TEXT NOT NULL,
    mime_type TEXT NOT NULL,
    alt TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS product_media (
    product_id TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    media_id TEXT NOT NULL REFERENCES media(id) ON DELETE CASCADE,
    position INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (product_id, media_id)
);

CREATE TABLE IF NOT EXISTS variants (
    id TEXT PRIMARY KEY,
    product_id TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    sku TEXT NOT NULL,
    options TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(options) AND json_type(options) = 'object'),
    UNIQUE (product_id, sku)
);

CREATE TABLE IF NOT EXISTS prices (
    variant_id TEXT NOT NULL REFERENCES variants(id) ON DELETE CASCADE,
    currency TEXT NOT NULL CHECK (length(currency) = 3),
    amount TEXT NOT NULL,
    compare_at TEXT,
    PRIMARY KEY (variant_id, currency)
);

CREATE TABLE IF NOT EXISTS inventory_items (
    variant_id TEXT PRIMARY KEY REFERENCES variants(id) ON DELETE CASCADE,
    quantity INTEGER NOT NULL CHECK (quantity >= 0),
    track INTEGER NOT NULL CHECK (track IN (0, 1))
);

CREATE TABLE IF NOT EXISTS carts (
    id TEXT PRIMARY KEY,
    shop_id TEXT NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    currency TEXT NOT NULL CHECK (length(currency) = 3),
    access_token_hash BLOB NOT NULL UNIQUE CHECK (length(access_token_hash) = 32),
    access_token_expires_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS cart_lines (
    cart_id TEXT NOT NULL REFERENCES carts(id) ON DELETE CASCADE,
    variant_id TEXT NOT NULL REFERENCES variants(id),
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    PRIMARY KEY (cart_id, variant_id)
);

CREATE TABLE IF NOT EXISTS customers (
    id TEXT PRIMARY KEY,
    shop_id TEXT NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    email_key TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    UNIQUE (shop_id, email_key)
);

CREATE TABLE IF NOT EXISTS orders (
    id TEXT PRIMARY KEY,
    shop_id TEXT NOT NULL REFERENCES shops(id),
    customer_id TEXT NOT NULL REFERENCES customers(id),
    number TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL CHECK (status IN ('pending', 'awaiting_payment', 'paid', 'fulfilling', 'fulfilled', 'cancelled', 'refunded')),
    currency TEXT NOT NULL CHECK (length(currency) = 3),
    subtotal TEXT NOT NULL,
    tax_total TEXT NOT NULL,
    shipping_total TEXT NOT NULL,
    grand_total TEXT NOT NULL,
    shipping_address TEXT NOT NULL CHECK (json_valid(shipping_address) AND json_type(shipping_address) = 'object'),
    placed_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS order_lines (
    order_id TEXT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    variant_id TEXT NOT NULL REFERENCES variants(id),
    sku TEXT NOT NULL,
    name TEXT NOT NULL,
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    unit_price TEXT NOT NULL,
    line_total TEXT NOT NULL
);
