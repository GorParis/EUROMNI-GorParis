CREATE TABLE brands (
  id         CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT (UUID()) PRIMARY KEY,
  shop_id    CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  slug       VARCHAR(128) NOT NULL,
  name       VARCHAR(200) NOT NULL,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE (shop_id, slug),
  CONSTRAINT fk_brands_shop_id FOREIGN KEY (shop_id) REFERENCES shops(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE suppliers (
  id         CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT (UUID()) PRIMARY KEY,
  shop_id    CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  code       VARCHAR(64) NOT NULL,
  name       VARCHAR(200) NOT NULL,
  active     BOOLEAN NOT NULL DEFAULT TRUE,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE (shop_id, code),
  CONSTRAINT fk_suppliers_shop_id FOREIGN KEY (shop_id) REFERENCES shops(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

ALTER TABLE products
  ADD COLUMN brand_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
  ADD COLUMN personalization_options JSON NOT NULL DEFAULT ('[]'),
  ADD CONSTRAINT fk_products_brand_id FOREIGN KEY (brand_id) REFERENCES brands(id) ON DELETE SET NULL;

CREATE TABLE product_suppliers (
  product_id  CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  supplier_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  PRIMARY KEY (product_id, supplier_id),
  CONSTRAINT fk_product_suppliers_product_id FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE,
  CONSTRAINT fk_product_suppliers_supplier_id FOREIGN KEY (supplier_id) REFERENCES suppliers(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE variant_suppliers (
  variant_id   CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  supplier_id  CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  supplier_sku VARCHAR(64) NOT NULL,
  PRIMARY KEY (variant_id, supplier_id),
  CONSTRAINT fk_variant_suppliers_variant_id FOREIGN KEY (variant_id) REFERENCES variants(id) ON DELETE CASCADE,
  CONSTRAINT fk_variant_suppliers_supplier_id FOREIGN KEY (supplier_id) REFERENCES suppliers(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

ALTER TABLE cart_lines
  ADD COLUMN personalizations JSON NOT NULL DEFAULT ('[]'),
  ADD COLUMN personalization_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL
    DEFAULT '44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a',
  DROP INDEX cart_id,
  ADD UNIQUE INDEX cart_lines_variant_personalization_idx (cart_id, variant_id, personalization_hash);

ALTER TABLE order_lines
  ADD COLUMN personalization_snapshot JSON NOT NULL DEFAULT ('[]');
