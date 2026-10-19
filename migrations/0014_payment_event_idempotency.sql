ALTER TABLE payments
  ADD COLUMN provider_event_id text;

CREATE UNIQUE INDEX payments_provider_event_id_unique
  ON payments (provider_event_id)
  WHERE provider_event_id IS NOT NULL;
