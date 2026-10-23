ALTER TABLE email_outbox
  DROP CONSTRAINT email_outbox_event_type_check,
  ADD CONSTRAINT email_outbox_event_type_check
    CHECK (event_type IN ('order_confirmation', 'order_status'));

CREATE TABLE order_lifecycle_events (
  id          CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT (UUID()) PRIMARY KEY,
  shop_id     CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  order_id    CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  event_type  VARCHAR(32) NOT NULL,
  source      VARCHAR(16) NOT NULL,
  tracking    VARCHAR(512),
  occurred_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE KEY order_lifecycle_events_order_type_unique (order_id, event_type),
  KEY order_lifecycle_events_order_idx (order_id, occurred_at, id),
  CONSTRAINT order_lifecycle_event_type_check CHECK (event_type IN (
    'order_placed', 'paid', 'processing', 'prepared', 'shipped', 'delivered'
  )),
  CONSTRAINT order_lifecycle_source_check CHECK (source IN (
    'checkout', 'payment', 'staff', 'supplier', 'carrier', 'local_demo'
  )),
  CONSTRAINT fk_order_lifecycle_shop FOREIGN KEY (shop_id) REFERENCES shops(id) ON DELETE CASCADE,
  CONSTRAINT fk_order_lifecycle_order FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
