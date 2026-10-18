package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"euromni/internal/dbdialect"
	"euromni/internal/seed"
)

var ErrEmptyCart = errors.New("cart is empty")
var ErrOrderTransition = errors.New("order status transition is not allowed")
var ErrShippingUnavailable = errors.New("shipping method unavailable")
var ErrShippingQuoteChanged = errors.New("shipping quote changed since it was displayed")

type CheckoutWriter interface {
	CheckoutCart(ctx context.Context, shopID string, tokenHash []byte, details CheckoutDetails) (*OrderView, error)
}

type OrderAdminReader interface {
	ListAdminOrders(ctx context.Context, shopID string, limit, offset int) ([]AdminOrderView, error)
	GetAdminOrder(ctx context.Context, shopID, orderID string) (*AdminOrderView, error)
	CancelAdminOrder(ctx context.Context, shopID, orderID string) error
	ConfirmManualOrderPayment(ctx context.Context, shopID, orderID, providerReference string) error
	FulfillAdminOrder(ctx context.Context, shopID, orderID, tracking string) error
}

type CheckoutDetails struct {
	Email                 string
	Name                  string
	ShippingCarrierCode   string
	ExpectedShippingTotal string
	ExpectedShippingQuoteID string
	ShippingAddress       ShippingAddress
}

type ShippingAddress struct {
	Line1      string `json:"line1"`
	Line2      string `json:"line2,omitempty"`
	City       string `json:"city"`
	PostalCode string `json:"postal_code"`
	Region     string `json:"region,omitempty"`
	Country    string `json:"country"`
}

type OrderView struct {
	ID                      string          `json:"id"`
	Number                  string          `json:"number"`
	Status                  string          `json:"status"`
	Currency                string          `json:"currency"`
	Subtotal                string          `json:"subtotal"`
	TaxTotal                string          `json:"tax_total"`
	ShippingTotal           string          `json:"shipping_total"`
	ShippingCarrierCode     string          `json:"shipping_carrier_code"`
	ShippingCarrierName     string          `json:"shipping_carrier_name"`
	ShippingRateMode        string          `json:"shipping_rate_mode"`
	ShippingWeightGrams     *int64          `json:"shipping_weight_grams,omitempty"`
	ShippingPricingSubtotal *string         `json:"shipping_pricing_subtotal,omitempty"`
	ShippingRateRule        json.RawMessage `json:"shipping_rate_rule,omitempty"`
	GrandTotal              string          `json:"grand_total"`
	Items                   []OrderLineView `json:"items"`
}

type OrderLineView struct {
	SKU       string `json:"sku"`
	Name      string `json:"name"`
	Quantity  int    `json:"quantity"`
	UnitPrice string `json:"unit_price"`
	LineTotal string `json:"line_total"`
}

type AdminOrderView struct {
	ID                      string          `json:"id"`
	Number                  string          `json:"number"`
	Status                  string          `json:"status"`
	Currency                string          `json:"currency"`
	Subtotal                string          `json:"subtotal"`
	TaxTotal                string          `json:"tax_total"`
	ShippingTotal           string          `json:"shipping_total"`
	ShippingCarrierCode     string          `json:"shipping_carrier_code"`
	ShippingCarrierName     string          `json:"shipping_carrier_name"`
	ShippingZoneCode        string          `json:"shipping_zone_code"`
	ShippingZoneName        string          `json:"shipping_zone_name"`
	ShippingRateMode        string          `json:"shipping_rate_mode"`
	ShippingWeightGrams     *int64          `json:"shipping_weight_grams,omitempty"`
	ShippingPricingSubtotal *string         `json:"shipping_pricing_subtotal,omitempty"`
	ShippingRateRule        json.RawMessage `json:"shipping_rate_rule,omitempty"`
	ShipmentStatus          string          `json:"shipment_status"`
	Tracking                string          `json:"tracking"`
	GrandTotal              string          `json:"grand_total"`
	Email                   string          `json:"email"`
	Name                    string          `json:"name"`
	ShippingAddress         ShippingAddress `json:"shipping_address"`
	CreatedAt               time.Time       `json:"created_at"`
	Items                   []OrderLineView `json:"items"`
}

type checkoutLine struct {
	variantID   string
	weightGrams sql.NullInt64
	OrderLineView
}

func newOrderIdentifiers() (string, string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", "", err
	}
	raw[6] = raw[6]&0x0f | 0x40
	raw[8] = raw[8]&0x3f | 0x80
	id := fmt.Sprintf("%x-%x-%x-%x-%x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16])
	return id, "EU-" + strings.ToUpper(hex.EncodeToString(raw[:10])), nil
}

func (s *Store) CheckoutCart(ctx context.Context, shopID string, tokenHash []byte, details CheckoutDetails) (*OrderView, error) {
	if s == nil || s.SQL == nil || len(tokenHash) != 32 {
		return nil, ErrInvalidCartToken
	}
	orderID, orderNumber, err := newOrderIdentifiers()
	if err != nil {
		return nil, err
	}
	addressJSON, err := json.Marshal(details.ShippingAddress)
	if err != nil {
		return nil, err
	}
	tx, err := s.SQL.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var cartID, currency, locale string
	err = tx.QueryRowContext(ctx, `
		SELECT c.id::text, c.currency::text, s.default_locale
		FROM carts c
		JOIN shops s ON s.id = c.shop_id
		WHERE c.shop_id::text = $1
		  AND c.access_token_hash = $2
		  AND c.access_token_expires_at > now()
		FOR UPDATE OF c
	`, shopID, tokenHash).Scan(&cartID, &currency, &locale)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidCartToken
	}
	if err != nil {
		return nil, err
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT v.id::text, v.sku, cl.quantity, COALESCE(pt.name, pr.slug), p.amount::text, v.weight_grams
		FROM cart_lines cl
		JOIN variants v ON v.id = cl.variant_id
		JOIN products pr ON pr.id = v.product_id
		JOIN shops s ON s.id = pr.shop_id
		JOIN prices p ON p.variant_id = v.id AND p.currency = $3
		LEFT JOIN product_translations pt ON pt.product_id = pr.id AND pt.locale = s.default_locale
		WHERE cl.cart_id::text = $1
		  AND pr.shop_id::text = $2
		  AND pr.status = 'active'
		ORDER BY v.id
		FOR UPDATE OF v
	`, cartID, shopID, currency)
	if err != nil {
		return nil, err
	}
	lines := make([]checkoutLine, 0)
	subtotal := new(big.Rat)
	var totalWeight int64
	weightKnown := true
	for rows.Next() {
		var line checkoutLine
		if err := rows.Scan(&line.variantID, &line.SKU, &line.Quantity, &line.Name, &line.UnitPrice, &line.weightGrams); err != nil {
			_ = rows.Close()
			return nil, err
		}
		unitPrice, ok := new(big.Rat).SetString(line.UnitPrice)
		if !ok {
			_ = rows.Close()
			return nil, errors.New("invalid product price")
		}
		lineAmount := new(big.Rat).Mul(unitPrice, big.NewRat(int64(line.Quantity), 1))
		line.LineTotal = lineAmount.FloatString(4)
		subtotal.Add(subtotal, lineAmount)
		if line.weightGrams.Valid && line.weightGrams.Int64 <= (1<<63-1)/int64(line.Quantity) {
			lineWeight := line.weightGrams.Int64 * int64(line.Quantity)
			if totalWeight > (1<<63-1)-lineWeight {
				weightKnown = false
			} else {
				totalWeight += lineWeight
			}
		} else {
			weightKnown = false
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return nil, ErrEmptyCart
	}
	var carrierName, zoneCode, zoneName, rateMode, shippingAmount string
	err = tx.QueryRowContext(ctx, `
		SELECT carrier.name, zone.code, zone.name, rate.rate_mode, rate.amount::text
		FROM shipping_rates rate
		JOIN shipping_carriers carrier
		  ON carrier.shop_id = rate.shop_id AND carrier.code = rate.carrier_code
		JOIN shipping_zones zone
		  ON zone.shop_id = rate.shop_id AND zone.code = rate.zone_code
		WHERE rate.shop_id = $1
		  AND rate.carrier_code = $2
		  AND rate.currency = $3
		  AND rate.active = true AND carrier.active = true AND zone.active = true
		  AND zone.country_codes @> jsonb_build_array($4::text)
	`, shopID, details.ShippingCarrierCode, currency, details.ShippingAddress.Country).Scan(
		&carrierName, &zoneCode, &zoneName, &rateMode, &shippingAmount,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrShippingUnavailable
	}
	if err != nil {
		return nil, err
	}
	shippingOption := ShippingOption{
		CarrierCode: details.ShippingCarrierCode, CarrierName: carrierName,
		ZoneCode: zoneCode, ZoneName: zoneName, Amount: shippingAmount,
		Currency: currency, RateMode: rateMode,
	}
	var snapshotRule json.RawMessage
	var snapshotWeight *int64
	var snapshotSubtotal *string
	if rateMode == "weight_value" {
		if !weightKnown {
			return nil, ErrShippingUnavailable
		}
		rule, err := matchingShippingRateRule(
			ctx, tx, shopID, details.ShippingCarrierCode, zoneCode, totalWeight, subtotal,
		)
		if err != nil {
			return nil, err
		}
		if rule == nil {
			return nil, ErrShippingUnavailable
		}
		shippingAmount = rule.Amount
		shippingOption.Amount = rule.Amount
		shippingOption.WeightGrams = &totalWeight
		shippingOption.PricingSubtotal = subtotal.FloatString(4)
		shippingOption.RateRule = rule
		ruleJSON, err := json.Marshal(rule)
		if err != nil {
			return nil, err
		}
		snapshotRule = ruleJSON
		snapshotWeight = &totalWeight
		pricingSubtotal := subtotal.FloatString(4)
		snapshotSubtotal = &pricingSubtotal
		if details.ExpectedShippingTotal == "" {
			return nil, ErrShippingQuoteChanged
		}
	} else if rateMode != "fixed" {
		return nil, fmt.Errorf("unsupported shipping rate mode %q", rateMode)
	}
	shippingTotal, ok := new(big.Rat).SetString(shippingAmount)
	if !ok {
		return nil, errors.New("invalid shipping rate")
	}
	if details.ExpectedShippingTotal != "" {
		expected, ok := new(big.Rat).SetString(details.ExpectedShippingTotal)
		if !ok || expected.Cmp(shippingTotal) != 0 {
			return nil, ErrShippingQuoteChanged
		}
	}
	if details.ExpectedShippingQuoteID != "" {
		quoteID, err := shippingOptionQuoteID(shippingOption)
		if err != nil {
			return nil, err
		}
		if quoteID != details.ExpectedShippingQuoteID {
			return nil, ErrShippingQuoteChanged
		}
	}
	var cartLineCount int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM cart_lines WHERE cart_id::text = $1`, cartID).Scan(&cartLineCount); err != nil {
		return nil, err
	}
	if cartLineCount != len(lines) {
		return nil, ErrNotFound
	}

	for _, line := range lines {
		var available int
		var tracked bool
		err := tx.QueryRowContext(ctx, `
			SELECT quantity, track
			FROM inventory_items
			WHERE variant_id::text = $1
			FOR UPDATE
		`, line.variantID).Scan(&available, &tracked)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrOutOfStock
		}
		if err != nil {
			return nil, err
		}
		if tracked && available < line.Quantity {
			return nil, ErrOutOfStock
		}
		if tracked {
			if _, err := tx.ExecContext(ctx, `
				UPDATE inventory_items
				SET quantity = quantity - $2, updated_at = now()
				WHERE variant_id::text = $1
			`, line.variantID, line.Quantity); err != nil {
				return nil, err
			}
		}
	}

	var customerID string
	if s.Dialect == dbdialect.MariaDB {
		customerID, err = NewCatalogID()
		if err != nil {
			return nil, err
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO customers (id, shop_id, email, name)
			VALUES ($1, $2, $3, $4)
			ON DUPLICATE KEY UPDATE name = VALUES(name), updated_at = now()
		`, customerID, shopID, details.Email, details.Name)
		if err != nil {
			return nil, err
		}
		err = tx.QueryRowContext(ctx, `
			SELECT id::text FROM customers WHERE shop_id = $1 AND email = $2
		`, shopID, details.Email).Scan(&customerID)
	} else {
		err = tx.QueryRowContext(ctx, `
			INSERT INTO customers (shop_id, email, name)
			VALUES ($1, $2, $3)
			ON CONFLICT (shop_id, email)
			DO UPDATE SET name = EXCLUDED.name, updated_at = now()
			RETURNING id::text
		`, shopID, details.Email, details.Name).Scan(&customerID)
	}
	if err != nil {
		return nil, err
	}
	totalWithShipping := new(big.Rat).Add(subtotal, shippingTotal)
	total := totalWithShipping.FloatString(4)
	shippingTotalText := shippingTotal.FloatString(4)
	persistedOrderID := orderID
	_, err = tx.ExecContext(ctx, `
		INSERT INTO orders (
			id, shop_id, customer_id, number, status, currency,
			subtotal, tax_total, shipping_total, grand_total,
			shipping_address, shipping_carrier_code, shipping_carrier_name,
			shipping_zone_code, shipping_zone_name, shipping_rate_mode,
			shipping_weight_grams, shipping_pricing_subtotal, shipping_rate_rule, placed_at
		)
		VALUES ($1, $2, $3, $4, 'pending', $5, $6, 0, $7, $8, $9::jsonb, $10, $11, $12, $13, $14, $15, $16, $17::jsonb, now())
	`, orderID, shopID, customerID, orderNumber, currency, subtotal.FloatString(4), shippingTotalText,
		total, string(addressJSON), details.ShippingCarrierCode, carrierName, zoneCode, zoneName,
		rateMode, snapshotWeight, snapshotSubtotal, nullableJSON(snapshotRule))
	if err != nil {
		return nil, err
	}
	for _, line := range lines {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO order_lines (order_id, variant_id, sku, name, quantity, unit_price, line_total)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, persistedOrderID, line.variantID, line.SKU, line.Name, line.Quantity, line.UnitPrice, line.LineTotal); err != nil {
			return nil, err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM cart_lines WHERE cart_id::text = $1`, cartID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &OrderView{
		ID: persistedOrderID, Number: orderNumber, Status: "pending", Currency: currency,
		Subtotal: subtotal.FloatString(4), TaxTotal: "0.0000", ShippingTotal: shippingTotalText,
		ShippingCarrierCode: details.ShippingCarrierCode, ShippingCarrierName: carrierName,
		ShippingRateMode: rateMode, ShippingWeightGrams: snapshotWeight,
		ShippingPricingSubtotal: snapshotSubtotal, ShippingRateRule: snapshotRule,
		GrandTotal: total,
		Items:      orderLines(lines),
	}, nil
}

func nullableJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return string(raw)
}

func orderLines(lines []checkoutLine) []OrderLineView {
	items := make([]OrderLineView, len(lines))
	for i := range lines {
		items[i] = lines[i].OrderLineView
	}
	return items
}

func (m *Memory) CheckoutCart(ctx context.Context, shopID string, tokenHash []byte, details CheckoutDetails) (*OrderView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if shopID != seed.ShopID || len(tokenHash) != 32 {
		return nil, ErrInvalidCartToken
	}
	orderID, orderNumber, err := newOrderIdentifiers()
	if err != nil {
		return nil, err
	}
	key := hex.EncodeToString(tokenHash)
	m.mu.Lock()
	defer m.mu.Unlock()
	cart, ok := m.carts[key]
	if !ok || !time.Now().Before(cart.expiresAt) {
		return nil, ErrInvalidCartToken
	}
	if len(cart.lines) == 0 {
		return nil, ErrEmptyCart
	}
	if details.ShippingCarrierCode != "colissimo" || details.ShippingAddress.Country != "FR" {
		return nil, ErrShippingUnavailable
	}
	shippingOption := ShippingOption{
		CarrierCode: "colissimo", CarrierName: "Colissimo (tarif de démonstration)",
		ZoneCode: "france-ue", ZoneName: "France et Union européenne",
		Amount: "6.9000", Currency: cart.currency, RateMode: "fixed",
	}
	if details.ExpectedShippingQuoteID != "" {
		quoteID, err := shippingOptionQuoteID(shippingOption)
		if err != nil {
			return nil, err
		}
		if quoteID != details.ExpectedShippingQuoteID {
			return nil, ErrShippingQuoteChanged
		}
	}
	variantIDs := make([]string, 0, len(cart.lines))
	for variantID := range cart.lines {
		variantIDs = append(variantIDs, variantID)
	}
	sort.Strings(variantIDs)
	lines := make([]checkoutLine, 0, len(variantIDs))
	subtotal := new(big.Rat)
	variants := make([]*VariantView, 0, len(variantIDs))
	for _, variantID := range variantIDs {
		product, variant := m.findVariant(shopID, variantID)
		if variant == nil {
			return nil, ErrNotFound
		}
		quantity := cart.lines[variantID]
		if variant.Inventory.Track && variant.Inventory.Quantity < quantity {
			return nil, ErrOutOfStock
		}
		price, ok := priceForCurrency(variant, cart.currency)
		if !ok {
			return nil, ErrNotFound
		}
		unitPrice, ok := new(big.Rat).SetString(price.Amount)
		if !ok {
			return nil, errors.New("invalid product price")
		}
		lineAmount := new(big.Rat).Mul(unitPrice, big.NewRat(int64(quantity), 1))
		lineName := product.Slug
		for _, translation := range product.Translations {
			if translation.Locale == "fr-FR" {
				lineName = translation.Name
				break
			}
		}
		lines = append(lines, checkoutLine{
			variantID: variantID,
			OrderLineView: OrderLineView{
				SKU: variant.SKU, Name: lineName, Quantity: quantity,
				UnitPrice: price.Amount, LineTotal: lineAmount.FloatString(4),
			},
		})
		variants = append(variants, variant)
		subtotal.Add(subtotal, lineAmount)
	}
	shippingTotal := "6.9000"
	shippingAmount, _ := new(big.Rat).SetString(shippingTotal)
	total := new(big.Rat).Add(subtotal, shippingAmount).FloatString(4)
	for i, variant := range variants {
		if variant.Inventory.Track {
			variant.Inventory.Quantity -= lines[i].Quantity
		}
	}
	clear(cart.lines)
	return &OrderView{
		ID: orderID, Number: orderNumber, Status: "pending", Currency: cart.currency,
		Subtotal: subtotal.FloatString(4), TaxTotal: "0.0000", ShippingTotal: shippingTotal,
		ShippingCarrierCode: "colissimo", ShippingCarrierName: "Colissimo (tarif de démonstration)",
		GrandTotal: total,
		Items:      orderLines(lines),
	}, nil
}
