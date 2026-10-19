ALTER TABLE orders
  ADD COLUMN shipping_policy_snapshot JSON NOT NULL DEFAULT ('{}');
