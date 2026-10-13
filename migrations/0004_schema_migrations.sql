BEGIN;

CREATE TABLE schema_migrations (
  version     text PRIMARY KEY,
  recorded_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO schema_migrations (version) VALUES
  ('0001_init.sql'),
  ('0002_seed_example.sql'),
  ('0003_cart_access_tokens.sql'),
  ('0004_schema_migrations.sql');

COMMIT;