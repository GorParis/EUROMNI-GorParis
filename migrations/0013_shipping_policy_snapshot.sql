ALTER TABLE orders
  ADD COLUMN shipping_policy_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb;
