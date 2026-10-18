package store

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"time"

	"euromni/internal/dbdialect"
	"euromni/internal/seed"
)

type ShippingConfigReader interface {
	GetShippingConfig(ctx context.Context, shopID string) (*ShippingConfig, error)
	ReplaceShippingConfig(ctx context.Context, shopID string, config ShippingConfig) error
}

type ShippingOptionReader interface {
	ListShippingOptions(ctx context.Context, shopID, country string) ([]ShippingOption, error)
}

type ShippingQuoteReader interface {
	ListCartShippingOptions(ctx context.Context, shopID string, tokenHash []byte, country string) ([]ShippingOption, error)
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
	CarrierCode string             `json:"carrier_code"`
	ZoneCode    string             `json:"zone_code"`
	Mode        string             `json:"mode"`
	Amount      string             `json:"amount"`
	Currency    string             `json:"currency"`
	Active      bool               `json:"active"`
	Rules       []ShippingRateRule `json:"rules,omitempty"`
}

type ShippingRateRule struct {
	MinWeightGrams int64   `json:"min_weight_grams"`
	MaxWeightGrams *int64  `json:"max_weight_grams"`
	MinSubtotal    string  `json:"min_subtotal"`
	MaxSubtotal    *string `json:"max_subtotal"`
	Amount         string  `json:"amount"`
}

type ShippingOption struct {
	CarrierCode     string            `json:"carrier_code"`
	CarrierName     string            `json:"carrier_name"`
	ZoneCode        string            `json:"zone_code"`
	ZoneName        string            `json:"zone_name"`
	Amount          string            `json:"amount"`
	Currency        string            `json:"currency"`
	RateMode        string            `json:"rate_mode,omitempty"`
	WeightGrams     *int64            `json:"weight_grams,omitempty"`
	PricingSubtotal string            `json:"pricing_subtotal,omitempty"`
	RateRule        *ShippingRateRule `json:"rate_rule,omitempty"`
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
		  AND rate.rate_mode = 'fixed'
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

func (s *Store) ListCartShippingOptions(ctx context.Context, shopID string, tokenHash []byte, country string) ([]ShippingOption, error) {
	if s == nil || s.SQL == nil || len(tokenHash) != 32 {
		return nil, ErrInvalidCartToken
	}
	var cartID, currency string
	err := s.SQL.QueryRowContext(ctx, `
		SELECT c.id::text, c.currency::text
		FROM carts c
		WHERE c.shop_id::text = $1
		  AND c.access_token_hash = $2
		  AND c.access_token_expires_at > now()
	`, shopID, tokenHash).Scan(&cartID, &currency)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidCartToken
	}
	if err != nil {
		return nil, err
	}

	rows, err := s.SQL.QueryContext(ctx, `
		SELECT cl.quantity, p.amount::text, v.weight_grams
		FROM cart_lines cl
		JOIN variants v ON v.id = cl.variant_id
		JOIN products pr ON pr.id = v.product_id
		JOIN prices p ON p.variant_id = v.id AND p.currency = $3
		WHERE cl.cart_id::text = $1 AND pr.shop_id::text = $2 AND pr.status = 'active'
		ORDER BY v.id
	`, cartID, shopID, currency)
	if err != nil {
		return nil, err
	}
	subtotal := new(big.Rat)
	var totalWeight int64
	weightKnown := true
	lineCount := 0
	for rows.Next() {
		var quantity int64
		var amount string
		var weight sql.NullInt64
		if err := rows.Scan(&quantity, &amount, &weight); err != nil {
			_ = rows.Close()
			return nil, err
		}
		unitAmount, ok := new(big.Rat).SetString(amount)
		if !ok {
			_ = rows.Close()
			return nil, errors.New("invalid product price")
		}
		subtotal.Add(subtotal, new(big.Rat).Mul(unitAmount, big.NewRat(quantity, 1)))
		if weight.Valid && weight.Int64 <= (1<<63-1)/quantity {
			lineWeight := weight.Int64 * quantity
			if totalWeight > (1<<63-1)-lineWeight {
				weightKnown = false
			} else {
				totalWeight += lineWeight
			}
		} else {
			weightKnown = false
		}
		lineCount++
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	var cartLineCount int
	if err := s.SQL.QueryRowContext(ctx, `
		SELECT count(*) FROM cart_lines WHERE cart_id::text = $1
	`, cartID).Scan(&cartLineCount); err != nil {
		return nil, err
	}
	if cartLineCount == 0 {
		return []ShippingOption{}, nil
	}
	if cartLineCount != lineCount {
		return nil, ErrNotFound
	}

	rateRows, err := s.SQL.QueryContext(ctx, `
		SELECT carrier.code, carrier.name, zone.code, zone.name,
		       rate.rate_mode, rate.amount::text, rate.currency::text
		FROM shipping_rates rate
		JOIN shipping_carriers carrier
		  ON carrier.shop_id = rate.shop_id AND carrier.code = rate.carrier_code
		JOIN shipping_zones zone
		  ON zone.shop_id = rate.shop_id AND zone.code = rate.zone_code
		WHERE rate.shop_id = $1
		  AND rate.active = true AND carrier.active = true AND zone.active = true
		  AND zone.country_codes @> jsonb_build_array($2::text)
		  AND rate.currency = $3
		ORDER BY carrier.name, carrier.code
	`, shopID, country, currency)
	if err != nil {
		return nil, err
	}
	type candidate struct {
		carrierCode, carrierName, zoneCode, zoneName, mode, amount, currency string
	}
	candidates := make([]candidate, 0)
	for rateRows.Next() {
		var entry candidate
		if err := rateRows.Scan(
			&entry.carrierCode, &entry.carrierName, &entry.zoneCode, &entry.zoneName,
			&entry.mode, &entry.amount, &entry.currency,
		); err != nil {
			_ = rateRows.Close()
			return nil, err
		}
		candidates = append(candidates, entry)
	}
	if err := rateRows.Err(); err != nil {
		_ = rateRows.Close()
		return nil, err
	}
	if err := rateRows.Close(); err != nil {
		return nil, err
	}

	options := make([]ShippingOption, 0, len(candidates))
	for _, entry := range candidates {
		option := ShippingOption{
			CarrierCode: entry.carrierCode, CarrierName: entry.carrierName,
			ZoneCode: entry.zoneCode, ZoneName: entry.zoneName,
			Currency: entry.currency, RateMode: entry.mode,
		}
		switch entry.mode {
		case "fixed":
			option.Amount = entry.amount
		case "weight_value":
			if !weightKnown {
				continue
			}
			rule, err := matchingShippingRateRule(
				ctx, s.SQL, shopID, entry.carrierCode, entry.zoneCode, totalWeight, subtotal,
			)
			if err != nil {
				return nil, err
			}
			if rule == nil {
				continue
			}
			option.Amount = rule.Amount
			option.WeightGrams = &totalWeight
			option.PricingSubtotal = subtotal.FloatString(4)
			option.RateRule = rule
		default:
			return nil, fmt.Errorf("unsupported shipping rate mode %q", entry.mode)
		}
		options = append(options, option)
	}
	return options, nil
}

type shippingRuleQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func matchingShippingRateRule(
	ctx context.Context, queryer shippingRuleQueryer, shopID, carrierCode, zoneCode string,
	weight int64, subtotal *big.Rat,
) (*ShippingRateRule, error) {
	rows, err := queryer.QueryContext(ctx, `
		SELECT min_weight_grams, max_weight_grams, min_subtotal::text, max_subtotal::text, amount::text
		FROM shipping_rate_rules
		WHERE shop_id = $1 AND carrier_code = $2 AND zone_code = $3
		ORDER BY min_weight_grams, min_subtotal
	`, shopID, carrierCode, zoneCode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var rule ShippingRateRule
		var maxWeight sql.NullInt64
		var minSubtotal string
		var maxSubtotal sql.NullString
		if err := rows.Scan(&rule.MinWeightGrams, &maxWeight, &minSubtotal, &maxSubtotal, &rule.Amount); err != nil {
			return nil, err
		}
		if maxWeight.Valid {
			rule.MaxWeightGrams = &maxWeight.Int64
		}
		rule.MinSubtotal = minSubtotal
		if maxSubtotal.Valid {
			rule.MaxSubtotal = &maxSubtotal.String
		}
		if weight < rule.MinWeightGrams || (rule.MaxWeightGrams != nil && weight >= *rule.MaxWeightGrams) {
			continue
		}
		minimum, ok := new(big.Rat).SetString(rule.MinSubtotal)
		if !ok {
			return nil, errors.New("invalid shipping subtotal bound")
		}
		if subtotal.Cmp(minimum) < 0 {
			continue
		}
		if rule.MaxSubtotal != nil {
			maximum, ok := new(big.Rat).SetString(*rule.MaxSubtotal)
			if !ok {
				return nil, errors.New("invalid shipping subtotal bound")
			}
			if subtotal.Cmp(maximum) >= 0 {
				continue
			}
		}
		return &rule, nil
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return nil, nil
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

func (m *Memory) ListCartShippingOptions(ctx context.Context, shopID string, tokenHash []byte, country string) ([]ShippingOption, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if shopID != seed.ShopID || len(tokenHash) != 32 {
		return nil, ErrInvalidCartToken
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	cart, ok := m.carts[hex.EncodeToString(tokenHash)]
	if !ok || !time.Now().Before(cart.expiresAt) {
		return nil, ErrInvalidCartToken
	}
	if country != "FR" || len(cart.lines) == 0 {
		return []ShippingOption{}, nil
	}
	return []ShippingOption{{
		CarrierCode: "colissimo",
		CarrierName: "Colissimo (tarif de démonstration)",
		ZoneCode:    "france-ue",
		ZoneName:    "France et Union européenne",
		Amount:      "6.9000",
		Currency:    cart.currency,
		RateMode:    "fixed",
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
		SELECT carrier_code, zone_code, rate_mode, amount::text, currency::text, active
		FROM shipping_rates
		WHERE shop_id = $1
		ORDER BY carrier_code, zone_code
	`, shopID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var rate ShippingRate
		if err := rows.Scan(&rate.CarrierCode, &rate.ZoneCode, &rate.Mode, &rate.Amount, &rate.Currency, &rate.Active); err != nil {
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
	for index := range config.Rates {
		rate := &config.Rates[index]
		rules, err := s.SQL.QueryContext(ctx, `
			SELECT min_weight_grams, max_weight_grams, min_subtotal::text, max_subtotal::text, amount::text
			FROM shipping_rate_rules
			WHERE shop_id = $1 AND carrier_code = $2 AND zone_code = $3
			ORDER BY min_weight_grams, min_subtotal
		`, shopID, rate.CarrierCode, rate.ZoneCode)
		if err != nil {
			return nil, err
		}
		rate.Rules = []ShippingRateRule{}
		for rules.Next() {
			var rule ShippingRateRule
			var maxWeight sql.NullInt64
			var maxSubtotal sql.NullString
			if err := rules.Scan(&rule.MinWeightGrams, &maxWeight, &rule.MinSubtotal, &maxSubtotal, &rule.Amount); err != nil {
				_ = rules.Close()
				return nil, err
			}
			if maxWeight.Valid {
				rule.MaxWeightGrams = &maxWeight.Int64
			}
			if maxSubtotal.Valid {
				rule.MaxSubtotal = &maxSubtotal.String
			}
			rate.Rules = append(rate.Rules, rule)
		}
		if err := rules.Err(); err != nil {
			_ = rules.Close()
			return nil, err
		}
		if err := rules.Close(); err != nil {
			return nil, err
		}
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
	if s.Dialect == dbdialect.MariaDB {
		var lockedShopID string
		if err := tx.QueryRowContext(ctx, `SELECT id FROM shops WHERE id = $1 FOR UPDATE`, shopID).Scan(&lockedShopID); err != nil {
			return err
		}
	} else {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, shopID); err != nil {
			return err
		}
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
		mode := rate.Mode
		if mode == "" {
			mode = "fixed"
		}
		amount := rate.Amount
		if mode == "weight_value" {
			amount = "0"
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO shipping_rates (shop_id, carrier_code, zone_code, rate_mode, amount, currency, active)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, shopID, rate.CarrierCode, rate.ZoneCode, mode, amount, rate.Currency, rate.Active); err != nil {
			return err
		}
		for _, rule := range rate.Rules {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO shipping_rate_rules (
					shop_id, carrier_code, zone_code, min_weight_grams, max_weight_grams,
					min_subtotal, max_subtotal, amount
				)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			`, shopID, rate.CarrierCode, rate.ZoneCode, rule.MinWeightGrams, rule.MaxWeightGrams,
				rule.MinSubtotal, rule.MaxSubtotal, rule.Amount); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

var _ ShippingConfigReader = (*Store)(nil)
var _ ShippingOptionReader = (*Store)(nil)
var _ ShippingOptionReader = (*Memory)(nil)
var _ ShippingQuoteReader = (*Store)(nil)
var _ ShippingQuoteReader = (*Memory)(nil)
