ALTER TABLE products
  ADD COLUMN volume_value bigint,
  ADD COLUMN volume_unit text,
  ADD CONSTRAINT products_volume_value_check
    CHECK (volume_value IS NULL OR volume_value >= 0);

ALTER TABLE inventory_items
  ADD COLUMN stock_status text NOT NULL DEFAULT 'in_stock',
  ADD CONSTRAINT inventory_items_stock_status_check
    CHECK (stock_status IN ('in_stock', 'out_of_stock'));

ALTER TABLE products
  ADD CONSTRAINT products_shop_id_id_unique UNIQUE (shop_id, id);

CREATE TABLE product_tags (
  product_id uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
  tag text NOT NULL CHECK (length(tag) BETWEEN 1 AND 128),
  PRIMARY KEY (product_id, tag)
);

CREATE TABLE supplier_product_refs (
  shop_id uuid NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
  provider text NOT NULL CHECK (provider = 'selfnamed'),
  external_id text NOT NULL CHECK (external_id ~ '^[1-9][0-9]{0,19}$'),
  product_id uuid NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (shop_id, provider, external_id),
  UNIQUE (product_id),
  FOREIGN KEY (shop_id, product_id) REFERENCES products(shop_id, id) ON DELETE CASCADE
);
