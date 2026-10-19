CREATE TABLE brands (
    id TEXT PRIMARY KEY,
    shop_id TEXT NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    slug TEXT NOT NULL,
    name TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE (shop_id, slug)
);

CREATE TABLE suppliers (
    id TEXT PRIMARY KEY,
    shop_id TEXT NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    active INTEGER NOT NULL DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE (shop_id, code)
);

ALTER TABLE products ADD COLUMN brand_id TEXT REFERENCES brands(id) ON DELETE SET NULL;
ALTER TABLE products ADD COLUMN personalization_options TEXT NOT NULL DEFAULT '[]';

CREATE TABLE product_suppliers (
    product_id TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    supplier_id TEXT NOT NULL REFERENCES suppliers(id) ON DELETE RESTRICT,
    PRIMARY KEY (product_id, supplier_id)
);

CREATE TABLE variant_suppliers (
    variant_id TEXT NOT NULL REFERENCES variants(id) ON DELETE CASCADE,
    supplier_id TEXT NOT NULL REFERENCES suppliers(id) ON DELETE RESTRICT,
    supplier_sku TEXT NOT NULL,
    PRIMARY KEY (variant_id, supplier_id)
);

CREATE TABLE cart_lines_v2 (
    cart_id TEXT NOT NULL REFERENCES carts(id) ON DELETE CASCADE,
    variant_id TEXT NOT NULL REFERENCES variants(id),
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    personalizations TEXT NOT NULL DEFAULT '[]',
    personalization_hash TEXT NOT NULL DEFAULT '44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a',
    PRIMARY KEY (cart_id, variant_id, personalization_hash)
);

INSERT INTO cart_lines_v2 (cart_id, variant_id, quantity)
SELECT cart_id, variant_id, quantity FROM cart_lines;
DROP TABLE cart_lines;
ALTER TABLE cart_lines_v2 RENAME TO cart_lines;

ALTER TABLE order_lines ADD COLUMN personalization_snapshot TEXT NOT NULL DEFAULT '[]';
