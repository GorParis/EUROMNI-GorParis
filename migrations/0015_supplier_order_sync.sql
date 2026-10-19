ALTER TABLE order_lines
  ADD COLUMN supplier_snapshot jsonb NOT NULL DEFAULT '[]'::jsonb;

CREATE TABLE supplier_order_syncs (
  order_id          uuid NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
  supplier_code     text NOT NULL,
  status            text NOT NULL CHECK (status IN ('syncing', 'synced', 'failed')),
  idempotency_key   text NOT NULL UNIQUE,
  external_order_id text,
  attempts          integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  last_error        text,
  locked_until      timestamptz,
  updated_at        timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (order_id, supplier_code)
);
