ALTER TABLE shipping_rates
  ADD CONSTRAINT chk_shipping_rates_rate_mode
    CHECK (rate_mode IN ('fixed', 'weight_value'));