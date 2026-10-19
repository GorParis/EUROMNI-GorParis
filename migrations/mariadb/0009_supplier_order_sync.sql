ALTER TABLE order_lines
  ADD COLUMN supplier_snapshot JSON NOT NULL DEFAULT ('[]');

CREATE TABLE supplier_order_syncs (
  order_id          CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  supplier_code     VARCHAR(128) NOT NULL,
  status            VARCHAR(16) NOT NULL,
  idempotency_key   VARCHAR(128) NOT NULL,
  external_order_id VARCHAR(512),
  attempts          integer NOT NULL DEFAULT 0,
  last_error        VARCHAR(1024),
  locked_until      DATETIME(6),
  updated_at        DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (order_id, supplier_code),
  UNIQUE KEY supplier_order_sync_idempotency_unique (idempotency_key),
  CONSTRAINT supplier_order_sync_status_check CHECK (status IN ('syncing', 'synced', 'failed')),
  CONSTRAINT supplier_order_sync_attempts_check CHECK (attempts >= 0),
  CONSTRAINT fk_supplier_order_sync_order FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
