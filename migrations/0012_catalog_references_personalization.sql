CREATE TABLE brands (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  shop_id    uuid NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
  slug       text NOT NULL,
  name       text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (shop_id, slug)
);

CREATE TABLE suppliers (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  shop_id    uuid NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
  code       text NOT NULL,
  name       text NOT NULL,
  active     boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (shop_id, code)
);

ALTER TABLE products
  ADD COLUMN brand_id uuid REFERENCES brands(id) ON DELETE SET NULL,
  ADD COLUMN personalization_options jsonb NOT NULL DEFAULT '[]'::jsonb;

CREATE TABLE product_suppliers (
  product_id  uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
  supplier_id uuid NOT NULL REFERENCES suppliers(id) ON DELETE RESTRICT,
  PRIMARY KEY (product_id, supplier_id)
);

CREATE TABLE variant_suppliers (
  variant_id   uuid NOT NULL REFERENCES variants(id) ON DELETE CASCADE,
  supplier_id  uuid NOT NULL REFERENCES suppliers(id) ON DELETE RESTRICT,
  supplier_sku text NOT NULL,
  PRIMARY KEY (variant_id, supplier_id)
);

ALTER TABLE cart_lines
  ADD COLUMN personalizations jsonb NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN personalization_hash char(64) NOT NULL
    DEFAULT '44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a';

ALTER TABLE cart_lines
  DROP CONSTRAINT cart_lines_cart_id_variant_id_key;

CREATE UNIQUE INDEX cart_lines_variant_personalization_idx
  ON cart_lines (cart_id, variant_id, personalization_hash);

ALTER TABLE order_lines
  ADD COLUMN personalization_snapshot jsonb NOT NULL DEFAULT '[]'::jsonb;
