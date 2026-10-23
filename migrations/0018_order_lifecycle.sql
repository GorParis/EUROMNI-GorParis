ALTER TABLE email_outbox
  DROP CONSTRAINT email_outbox_event_type_check,
  ADD CONSTRAINT email_outbox_event_type_check
    CHECK (event_type IN ('order_confirmation', 'order_status'));

CREATE TABLE order_lifecycle_events (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  shop_id     uuid NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
  order_id    uuid NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
  event_type  text NOT NULL CHECK (event_type IN (
    'order_placed', 'paid', 'processing', 'prepared', 'shipped', 'delivered'
  )),
  source      text NOT NULL CHECK (source IN (
    'checkout', 'payment', 'staff', 'supplier', 'carrier', 'local_demo'
  )),
  tracking    text,
  occurred_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (order_id, event_type)
);

CREATE INDEX order_lifecycle_events_order_idx
  ON order_lifecycle_events (order_id, occurred_at, id);
