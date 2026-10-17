package store

import (
	"context"
	"encoding/json"
	"errors"

	"euromni/internal/seed"
)

type ShippingConfigReader interface {
	GetShippingConfig(ctx context.Context, shopID string) (*ShippingConfig, error)
	ReplaceShippingConfig(ctx context.Context, shopID string, config ShippingConfig) error
}

type ShippingOptionReader interface {
	ListShippingOptions(ctx context.Context, shopID, country string) ([]ShippingOption, error)
}

type ShippingConfig struct {
	Carriers []ShippingCarrier `json:"carriers"`
	Zones    []ShippingZone    `json:"zones"`
	Rates    []ShippingRate    `json:"rates"`
}

type ShippingCarrier struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

type ShippingZone struct {
	Code         string   `json:"code"`
	Name         string   `json:"name"`
	CountryCodes []string `json:"country_codes"`
	Active       bool     `json:"active"`
}

type ShippingRate struct {
	CarrierCode string `json:"carrier_code"`
	ZoneCode    string `json:"zone_code"`
	Amount      string `json:"amount"`
	Currency    string `json:"currency"`
	Active      bool   `json:"active"`
}

type ShippingOption struct {
	CarrierCode string `json:"carrier_code"`
	CarrierName string `json:"carrier_name"`
	ZoneCode    string `json:"zone_code"`
	ZoneName    string `json:"zone_name"`
	Amount      string `json:"amount"`
	Currency    string `json:"currency"`
}

func (s *Store) ListShippingOptions(ctx context.Context, shopID, country string) ([]ShippingOption, error) {
	if s == nil || s.SQL == nil {
		return nil, errors.New("store not configured")
	}
	rows, err := s.SQL.QueryContext(ctx, `
		SELECT carrier.code, carrier.name, zone.code, zone.name, rate.amount::text, rate.currency::text
		FROM shipping_rates rate
		JOIN shipping_carriers carrier
		  ON carrier.shop_id = rate.shop_id AND carrier.code = rate.carrier_code
		JOIN shipping_zones zone
		  ON zone.shop_id = rate.shop_id AND zone.code = rate.zone_code
		JOIN shops shop ON shop.id = rate.shop_id
		WHERE rate.shop_id = $1
		  AND rate.active = true AND carrier.active = true AND zone.active = true
		  AND zone.country_codes @> jsonb_build_array($2::text)
		  AND rate.currency = shop.currency
		ORDER BY rate.amount, carrier.name, carrier.code
	`, shopID, country)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	options := make([]ShippingOption, 0)
	for rows.Next() {
		var option ShippingOption
		if err := rows.Scan(
			&option.CarrierCode, &option.CarrierName, &option.ZoneCode,
			&option.ZoneName, &option.Amount, &option.Currency,
		); err != nil {
			return nil, err
		}
		options = append(options, option)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return options, nil
}

func (m *Memory) ListShippingOptions(ctx context.Context, shopID, country string) ([]ShippingOption, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if shopID != seed.ShopID || country != "FR" {
		return []ShippingOption{}, nil
	}
	return []ShippingOption{{
		CarrierCode: "colissimo",
		CarrierName: "Colissimo (tarif de démonstration)",
		ZoneCode:    "france-ue",
		ZoneName:    "France et Union européenne",
		Amount:      "6.9000",
		Currency:    "EUR",
	}}, nil
}

func (s *Store) GetShippingConfig(ctx context.Context, shopID string) (*ShippingConfig, error) {
	if s == nil || s.SQL == nil {
		return nil, errors.New("store not configured")
	}
	config := &ShippingConfig{
		Carriers: []ShippingCarrier{},
		Zones:    []ShippingZone{},
		Rates:    []ShippingRate{},
	}
	rows, err := s.SQL.QueryContext(ctx, `
		SELECT code, name, active
		FROM shipping_carriers
		WHERE shop_id = $1
		ORDER BY name, code
	`, shopID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var carrier ShippingCarrier
		if err := rows.Scan(&carrier.Code, &carrier.Name, &carrier.Active); err != nil {
			_ = rows.Close()
			return nil, err
		}
		config.Carriers = append(config.Carriers, carrier)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	rows, err = s.SQL.QueryContext(ctx, `
		SELECT code, name, country_codes::text, active
		FROM shipping_zones
		WHERE shop_id = $1
		ORDER BY name, code
	`, shopID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var zone ShippingZone
		var countries string
		if err := rows.Scan(&zone.Code, &zone.Name, &countries, &zone.Active); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := json.Unmarshal([]byte(countries), &zone.CountryCodes); err != nil {
			_ = rows.Close()
			return nil, err
		}
		config.Zones = append(config.Zones, zone)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	rows, err = s.SQL.QueryContext(ctx, `
		SELECT carrier_code, zone_code, amount::text, currency::text, active
		FROM shipping_rates
		WHERE shop_id = $1
		ORDER BY carrier_code, zone_code
	`, shopID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var rate ShippingRate
		if err := rows.Scan(&rate.CarrierCode, &rate.ZoneCode, &rate.Amount, &rate.Currency, &rate.Active); err != nil {
			_ = rows.Close()
			return nil, err
		}
		config.Rates = append(config.Rates, rate)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return config, nil
}

func (s *Store) ReplaceShippingConfig(ctx context.Context, shopID string, config ShippingConfig) error {
	if s == nil || s.SQL == nil {
		return errors.New("store not configured")
	}
	tx, err := s.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, shopID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM shipping_carriers WHERE shop_id = $1`, shopID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM shipping_zones WHERE shop_id = $1`, shopID); err != nil {
		return err
	}
	for _, carrier := range config.Carriers {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO shipping_carriers (shop_id, code, name, active)
			VALUES ($1, $2, $3, $4)
		`, shopID, carrier.Code, carrier.Name, carrier.Active); err != nil {
			return err
		}
	}
	for _, zone := range config.Zones {
		countries, err := json.Marshal(zone.CountryCodes)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO shipping_zones (shop_id, code, name, country_codes, active)
			VALUES ($1, $2, $3, $4::jsonb, $5)
		`, shopID, zone.Code, zone.Name, string(countries), zone.Active); err != nil {
			return err
		}
	}
	for _, rate := range config.Rates {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO shipping_rates (shop_id, carrier_code, zone_code, amount, currency, active)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, shopID, rate.CarrierCode, rate.ZoneCode, rate.Amount, rate.Currency, rate.Active); err != nil {
			return err
		}
	}
	return tx.Commit()
}

var _ ShippingConfigReader = (*Store)(nil)
var _ ShippingOptionReader = (*Store)(nil)
var _ ShippingOptionReader = (*Memory)(nil)
