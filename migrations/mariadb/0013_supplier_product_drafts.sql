ALTER TABLE products
  ADD COLUMN volume_value BIGINT,
  ADD COLUMN volume_unit VARCHAR(32),
  ADD CONSTRAINT products_volume_value_check
    CHECK (volume_value IS NULL OR volume_value >= 0),
  ADD CONSTRAINT products_shop_id_id_unique UNIQUE (shop_id, id);

ALTER TABLE inventory_items
  ADD COLUMN stock_status ENUM('in_stock','out_of_stock') NOT NULL DEFAULT 'in_stock';

CREATE TABLE product_tags (
  product_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  tag VARCHAR(128) NOT NULL,
  PRIMARY KEY (product_id, tag),
  CONSTRAINT fk_product_tags_product_id FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE supplier_product_refs (
  shop_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  provider VARCHAR(32) NOT NULL,
  external_id VARCHAR(20) NOT NULL,
  product_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (shop_id, provider, external_id),
  UNIQUE KEY supplier_product_refs_product_unique (product_id),
  CONSTRAINT supplier_product_refs_provider_check CHECK (provider = 'selfnamed'),
  CONSTRAINT supplier_product_refs_external_id_check CHECK (external_id REGEXP '^[1-9][0-9]{0,19}$'),
  CONSTRAINT fk_supplier_product_refs_shop_id FOREIGN KEY (shop_id) REFERENCES shops(id) ON DELETE CASCADE,
  CONSTRAINT fk_supplier_product_refs_product FOREIGN KEY (shop_id, product_id) REFERENCES products(shop_id, id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
