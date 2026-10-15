ALTER TABLE carts
  ADD COLUMN access_token_hash bytea,
  ADD COLUMN access_token_expires_at timestamptz NOT NULL DEFAULT now() + interval '30 days';

CREATE UNIQUE INDEX carts_access_token_hash_idx
  ON carts (access_token_hash)
  WHERE access_token_hash IS NOT NULL;
