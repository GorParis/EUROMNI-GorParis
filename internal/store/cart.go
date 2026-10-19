package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
)

var (
	ErrInvalidCartToken    = errors.New("invalid cart token")
	ErrInvalidCartQuantity = errors.New("invalid cart quantity")
	ErrOutOfStock          = errors.New("insufficient inventory")
)

const maxCartLineQuantity = 99

type CartReader interface {
	CreateCart(ctx context.Context, shopID string, tokenHash []byte) error
	GetCart(ctx context.Context, shopID string, tokenHash []byte) (*CartView, error)
	AddCartLine(ctx context.Context, shopID string, tokenHash []byte, variantID string, quantity int, selections ...map[string]string) (*CartView, error)
}

type CartView struct {
	Items []CartLineView `json:"items"`
	Total CartTotalView  `json:"total"`
}

type CartLineView struct {
	VariantID        string                     `json:"variant_id"`
	SKU              string                     `json:"sku"`
	Quantity         int                        `json:"quantity"`
	UnitAmount       string                     `json:"unit_amount"`
	Personalizations []PersonalizationSelection `json:"personalizations"`
}

type CartTotalView struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

func (s *Store) CreateCart(ctx context.Context, shopID string, tokenHash []byte) error {
	if s == nil || s.SQL == nil || len(tokenHash) != sha256.Size {
		return ErrInvalidCartToken
	}
	result, err := s.SQL.ExecContext(ctx, `
		INSERT INTO carts (shop_id, currency, access_token_hash)
		SELECT id, currency, $2
		FROM shops
		WHERE id::text = $1
	`, shopID, tokenHash)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetCart(ctx context.Context, shopID string, tokenHash []byte) (*CartView, error) {
	if s == nil || s.SQL == nil || len(tokenHash) != sha256.Size {
		return nil, ErrInvalidCartToken
	}
	var cartID, currency string
	err := s.SQL.QueryRowContext(ctx, `
		SELECT id::text, currency::text
		FROM carts
		WHERE shop_id::text = $1
		  AND access_token_hash = $2
		  AND access_token_expires_at > now()
	`, shopID, tokenHash).Scan(&cartID, &currency)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidCartToken
	}
	if err != nil {
		return nil, err
	}

	rows, err := s.SQL.QueryContext(ctx, `
		SELECT v.id::text, v.sku, cl.quantity, p.amount::text, cl.personalizations
		FROM cart_lines cl
		JOIN variants v ON v.id = cl.variant_id
		JOIN products pr ON pr.id = v.product_id
		JOIN prices p ON p.variant_id = v.id AND p.currency = $3
		WHERE cl.cart_id::text = $1
		  AND pr.shop_id::text = $2
		  AND pr.status = 'active'
		ORDER BY v.sku, v.id
	`, cartID, shopID, currency)
	if err != nil {
		return nil, err
	}
	items := make([]CartLineView, 0)
	for rows.Next() {
		var item CartLineView
		var personalizations []byte
		if err := rows.Scan(&item.VariantID, &item.SKU, &item.Quantity, &item.UnitAmount, &personalizations); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := json.Unmarshal(personalizations, &item.Personalizations); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if item.Personalizations == nil {
			item.Personalizations = []PersonalizationSelection{}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	var amount string
	err = s.SQL.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(p.amount * cl.quantity), 0)::numeric(19,4)::text
		FROM cart_lines cl
		JOIN variants v ON v.id = cl.variant_id
		JOIN products pr ON pr.id = v.product_id
		JOIN prices p ON p.variant_id = v.id AND p.currency = $3
		WHERE cl.cart_id::text = $1
		  AND pr.shop_id::text = $2
		  AND pr.status = 'active'
	`, cartID, shopID, currency).Scan(&amount)
	if err != nil {
		return nil, err
	}
	return &CartView{Items: items, Total: CartTotalView{Amount: amount, Currency: currency}}, nil
}

func (s *Store) AddCartLine(ctx context.Context, shopID string, tokenHash []byte, variantID string, quantity int, selectionsInput ...map[string]string) (*CartView, error) {
	if s == nil || s.SQL == nil || len(tokenHash) != sha256.Size {
		return nil, ErrInvalidCartToken
	}
	if quantity < 1 || quantity > maxCartLineQuantity {
		return nil, ErrInvalidCartQuantity
	}
	submitted := map[string]string{}
	if len(selectionsInput) > 0 && selectionsInput[0] != nil {
		submitted = selectionsInput[0]
	}
	tx, err := s.SQL.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var cartID, currency string
	err = tx.QueryRowContext(ctx, `
		SELECT id::text, currency::text
		FROM carts
		WHERE shop_id::text = $1
		  AND access_token_hash = $2
		  AND access_token_expires_at > now()
		FOR UPDATE
	`, shopID, tokenHash).Scan(&cartID, &currency)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidCartToken
	}
	if err != nil {
		return nil, err
	}

	var inventoryQuantity int
	var inventoryTracked bool
	var optionJSON []byte
	err = tx.QueryRowContext(ctx, `
		SELECT COALESCE(i.quantity, 0), COALESCE(i.track, true), pr.personalization_options
		FROM variants v
		JOIN products pr ON pr.id = v.product_id
		JOIN prices p ON p.variant_id = v.id AND p.currency = $3
		LEFT JOIN inventory_items i ON i.variant_id = v.id
		WHERE v.id::text = $2
		  AND pr.shop_id::text = $1
		  AND pr.status = 'active'
		FOR UPDATE OF v
	`, shopID, variantID, currency).Scan(&inventoryQuantity, &inventoryTracked, &optionJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	options, err := decodePersonalizationOptions(optionJSON)
	if err != nil {
		return nil, err
	}
	_, snapshot, personalizationHash, err := ValidatePersonalizationSelections(options, submitted)
	if err != nil {
		return nil, err
	}
	var currentQuantity int
	err = tx.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(quantity), 0) FROM cart_lines
		WHERE cart_id::text = $1 AND variant_id::text = $2
	`, cartID, variantID).Scan(&currentQuantity)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if currentQuantity+quantity > maxCartLineQuantity {
		return nil, ErrInvalidCartQuantity
	}
	if inventoryTracked && currentQuantity+quantity > inventoryQuantity {
		return nil, ErrOutOfStock
	}

	var snapshotLineQuantity int
	err = tx.QueryRowContext(ctx, `
		SELECT quantity FROM cart_lines
		WHERE cart_id::text = $1 AND variant_id::text = $2 AND personalization_hash = $3
	`, cartID, variantID, personalizationHash).Scan(&snapshotLineQuantity)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if snapshotLineQuantity+quantity > maxCartLineQuantity {
		return nil, ErrInvalidCartQuantity
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO cart_lines (cart_id, variant_id, quantity, personalizations, personalization_hash)
		VALUES ($1, $2, $3, $4::jsonb, $5)
		ON CONFLICT (cart_id, variant_id, personalization_hash)
		DO UPDATE SET quantity = cart_lines.quantity + EXCLUDED.quantity
	`, cartID, variantID, quantity, snapshot, personalizationHash)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetCart(ctx, shopID, tokenHash)
}
