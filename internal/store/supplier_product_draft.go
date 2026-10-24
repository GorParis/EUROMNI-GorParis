package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	supplierProductIDPattern = regexp.MustCompile(`^[1-9][0-9]{0,19}$`)
	supplierProductPrice     = regexp.MustCompile(`^(?:0|[1-9][0-9]{0,11})(?:\.[0-9]{1,4})?$`)
	supplierLocalePattern    = regexp.MustCompile(`^[A-Za-z]{2,8}(?:-[A-Za-z0-9]{1,8})*$`)
	supplierCurrencyPattern  = regexp.MustCompile(`^[A-Z]{3}$`)
)

func ValidateSupplierProductDraftInput(input SupplierProductDraftInput) error {
	if input.Provider != "selfnamed" || !supplierProductIDPattern.MatchString(input.ExternalID) ||
		input.PublicationStatus != "draft" {
		return errors.New("supplier product identity or publication status is invalid")
	}
	if strings.TrimSpace(input.Name) == "" || input.Name != strings.TrimSpace(input.Name) ||
		len(input.Name) > 512 || len(input.Description) > 8192 {
		return errors.New("supplier product text is invalid")
	}
	if input.Locale != nil || input.Price.Currency != nil || input.Weight.Unit != nil || input.Volume.Unit != nil ||
		input.Inventory.Track == nil || *input.Inventory.Track || input.Weight.Grams == nil || input.Volume.Value == nil ||
		!supplierProductPrice.MatchString(input.Price.Amount) ||
		(input.Inventory.Status != "instock" && input.Inventory.Status != "outofstock") {
		return errors.New("supplier product contains unsupported or unresolved source fields")
	}
	if *input.Weight.Grams < 0 || *input.Weight.Grams > 100_000_000 ||
		*input.Volume.Value < 0 || *input.Volume.Value > 1_000_000_000 {
		return errors.New("supplier product dimensions are invalid")
	}
	if input.Tags == nil || len(input.Tags) > 64 {
		return errors.New("supplier product tags are invalid")
	}
	for _, tag := range input.Tags {
		if tag == "" || tag != strings.TrimSpace(tag) || len(tag) > 128 {
			return errors.New("supplier product tags are invalid")
		}
	}
	if len(input.Unresolved) != 3 ||
		input.Unresolved[0] != "currency" ||
		input.Unresolved[1] != "locale" ||
		input.Unresolved[2] != "volume_unit" {
		return errors.New("supplier product unresolved fields are invalid")
	}
	return nil
}

func (s *Store) StageSupplierProductDraft(
	ctx context.Context,
	shopID string,
	input SupplierProductDraftInput,
) (*SupplierProductDraftResult, error) {
	if s == nil || s.SQL == nil {
		return nil, errors.New("store not configured")
	}
	if err := ValidateSupplierProductDraftInput(input); err != nil {
		return nil, err
	}

	tx, err := s.SQL.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var locale, currency string
	if err := tx.QueryRowContext(ctx, `
		SELECT default_locale, currency::text FROM shops WHERE id = $1 FOR UPDATE
	`, shopID).Scan(&locale, &currency); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	if locale == "" || len(locale) > 35 || !supplierLocalePattern.MatchString(locale) ||
		!supplierCurrencyPattern.MatchString(currency) {
		return nil, errors.New("shop locale or currency configuration is invalid")
	}

	var productID, status string
	err = tx.QueryRowContext(ctx, `
		SELECT ref.product_id::text, product.status::text
		FROM supplier_product_refs ref
		JOIN products product ON product.id = ref.product_id AND product.shop_id = ref.shop_id
		WHERE ref.shop_id = $1 AND ref.provider = $2 AND ref.external_id = $3
		FOR UPDATE OF ref, product
	`, shopID, input.Provider, input.ExternalID).Scan(&productID, &status)
	created := errors.Is(err, sql.ErrNoRows)
	if err != nil && !created {
		return nil, err
	}
	if !created && status != "draft" {
		return nil, fmt.Errorf("%w: supplier product is no longer a draft", ErrConflict)
	}

	stockStatus := "in_stock"
	if input.Inventory.Status == "outofstock" {
		stockStatus = "out_of_stock"
	}
	weight := *input.Weight.Grams
	productInput := ProductInput{
		Slug:   "selfnamed-" + input.ExternalID,
		Status: "draft",
		Volume: &ProductVolume{Value: *input.Volume.Value},
		Tags:   append([]string(nil), input.Tags...),
		Translations: []TranslationView{{
			Locale: locale, Name: input.Name, Description: input.Description,
		}},
		Variants: []VariantInput{{
			SKU:         "SN-" + input.ExternalID,
			Options:     map[string]any{},
			WeightGrams: &weight,
			Prices:      []PriceView{{Currency: currency, Amount: input.Price.Amount}},
			Inventory:   InventoryView{StockStatus: stockStatus},
		}},
	}
	productID, err = writeProductTx(ctx, tx, shopID, productID, productInput)
	if err != nil {
		return nil, err
	}
	if created {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO supplier_product_refs (shop_id, provider, external_id, product_id)
			VALUES ($1, $2, $3, $4)
		`, shopID, input.Provider, input.ExternalID, productID); err != nil {
			return nil, catalogDBError(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, catalogDBError(err)
	}

	product, err := s.readAdminProduct(ctx, shopID, productID)
	if err != nil {
		return nil, err
	}
	return &SupplierProductDraftResult{Product: *product, Created: created}, nil
}
