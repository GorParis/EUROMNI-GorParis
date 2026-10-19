ALTER TABLE payments
  ADD COLUMN provider_event_id VARCHAR(512) NULL;

CREATE UNIQUE INDEX payments_provider_event_id_unique
  ON payments (provider_event_id);
