package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrNotFound = errors.New("not found")

type ProductView struct {
	ID                     string                  `json:"id"`
	Slug                   string                  `json:"slug"`
	Status                 string                  `json:"status"`
	Brand                  *BrandView              `json:"brand,omitempty"`
	SupplierIDs            []string                `json:"-"`
	PersonalizationOptions []PersonalizationOption `json:"personalization_options"`
	Translations           []TranslationView       `json:"translations"`
	CategoryIDs            []string                `json:"category_ids"`
	Media                  []MediaView             `json:"media"`
	Variants               []VariantView           `json:"variants"`
}

type PersonalizationOption struct {
	Code     string                  `json:"code"`
	Label    string                  `json:"label"`
	Required bool                    `json:"required"`
	Choices  []PersonalizationChoice `json:"choices"`
}

type PersonalizationChoice struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type PersonalizationSelection struct {
	Code        string `json:"code"`
	Label       string `json:"label"`
	Value       string `json:"value"`
	ChoiceLabel string `json:"choice_label"`
}

type TranslationView struct {
	Locale      string `json:"locale"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type MediaView struct {
	ID       string `json:"id"`
	Position int    `json:"position"`
	URI      string `json:"uri"`
	MimeType string `json:"mime_type"`
	Alt      string `json:"alt"`
}

type VariantView struct {
	ID          string                `json:"id"`
	SKU         string                `json:"sku"`
	Options     map[string]any        `json:"options"`
	WeightGrams *int64                `json:"weight_grams,omitempty"`
	Prices      []PriceView           `json:"prices"`
	Inventory   InventoryView         `json:"inventory"`
	Suppliers   []VariantSupplierView `json:"-"`
}

type AdminProductView struct {
	ProductView
	SupplierIDs []string           `json:"supplier_ids"`
	Variants    []AdminVariantView `json:"variants"`
}

type AdminVariantView struct {
	VariantView
	Suppliers []VariantSupplierView `json:"suppliers"`
}

func AdminProduct(product ProductView) AdminProductView {
	admin := AdminProductView{
		ProductView: product,
		SupplierIDs: append([]string(nil), product.SupplierIDs...),
		Variants:    make([]AdminVariantView, len(product.Variants)),
	}
	for i, variant := range product.Variants {
		admin.Variants[i] = AdminVariantView{
			VariantView: variant,
			Suppliers:   append([]VariantSupplierView(nil), variant.Suppliers...),
		}
	}
	return admin
}

type VariantSupplierView struct {
	SupplierID   string `json:"supplier_id"`
	SupplierName string `json:"supplier_name"`
	SupplierSKU  string `json:"supplier_sku"`
}

type PriceView struct {
	Currency  string  `json:"currency"`
	Amount    string  `json:"amount"`
	CompareAt *string `json:"compare_at"`
}

type InventoryView struct {
	Quantity int  `json:"quantity"`
	Track    bool `json:"track"`
}

type ProductReader interface {
	GetProduct(ctx context.Context, shopID, productID string) (*ProductView, error)
	ListProducts(ctx context.Context, shopID string) ([]ProductView, error)
}

type ProductCategoryReader interface {
	ListProductsByCategory(ctx context.Context, shopID, categoryID string) ([]ProductView, error)
}

type NewProductsReader interface {
	ListNewProducts(ctx context.Context, shopID string, days, limit int) ([]ProductView, error)
}

var (
	_ ProductReader         = (*Store)(nil)
	_ ProductCategoryReader = (*Store)(nil)
	_ NewProductsReader     = (*Store)(nil)
)

func (s *Store) ListProducts(ctx context.Context, shopID string) ([]ProductView, error) {
	if s == nil || s.SQL == nil {
		return nil, errors.New("store not configured")
	}

	rows, err := s.SQL.QueryContext(ctx, `
		SELECT id::text
		FROM products
		WHERE shop_id = $1 AND status = 'active'
		ORDER BY slug, id
	`, shopID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	products := make([]ProductView, 0, len(ids))
	for _, id := range ids {
		product, err := s.GetProduct(ctx, shopID, id)
		if err != nil {
			return nil, err
		}
		products = append(products, *product)
	}
	return products, nil
}

func ValidateNewProductsQuery(days, limit int) error {
	if days < 0 || days > 3650 {
		return errors.New("new product window must be between 0 and 3650 days")
	}
	if limit < 1 || limit > 100 {
		return errors.New("new product limit must be between 1 and 100")
	}
	return nil
}

func (s *Store) ListNewProducts(ctx context.Context, shopID string, days, limit int) ([]ProductView, error) {
	if s == nil || s.SQL == nil {
		return nil, errors.New("store not configured")
	}
	if err := ValidateNewProductsQuery(days, limit); err != nil {
		return nil, err
	}
	if days == 0 {
		return []ProductView{}, nil
	}

	cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	rows, err := s.SQL.QueryContext(ctx, `
		SELECT id::text
		FROM products
		WHERE shop_id = $1 AND status = 'active' AND created_at >= $2
		ORDER BY created_at DESC, id DESC
		LIMIT $3
	`, shopID, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	products := make([]ProductView, 0, len(ids))
	for _, id := range ids {
		product, err := s.GetProduct(ctx, shopID, id)
		if err != nil {
			return nil, fmt.Errorf("load new product %s: %w", id, err)
		}
		products = append(products, *product)
	}
	return products, nil
}

func (s *Store) ListProductsByCategory(ctx context.Context, shopID, categoryID string) ([]ProductView, error) {
	if s == nil || s.SQL == nil {
		return nil, errors.New("store not configured")
	}

	rows, err := s.SQL.QueryContext(ctx, `
		SELECT p.id::text
		FROM products p
		JOIN product_categories pc ON pc.product_id = p.id
		WHERE p.shop_id = $1 AND p.status = 'active' AND pc.category_id = $2
		ORDER BY p.slug, p.id
	`, shopID, categoryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	products := make([]ProductView, 0, len(ids))
	for _, id := range ids {
		product, err := s.GetProduct(ctx, shopID, id)
		if err != nil {
			return nil, err
		}
		products = append(products, *product)
	}
	return products, nil
}

func (s *Store) GetProduct(ctx context.Context, shopID, productID string) (*ProductView, error) {
	return s.getProduct(ctx, shopID, productID, true)
}

func (s *Store) getProduct(ctx context.Context, shopID, productID string, activeOnly bool) (*ProductView, error) {
	if s == nil || s.SQL == nil {
		return nil, errors.New("store not configured")
	}

	var p ProductView
	var brandID sql.NullString
	var personalizationOptions []byte
	query := `
		SELECT id::text, slug, status::text, brand_id::text, personalization_options
		FROM products
		WHERE shop_id = $1 AND id = $2
	`
	if activeOnly {
		query += ` AND status = 'active'`
	}
	err := s.SQL.QueryRowContext(ctx, query, shopID, productID).Scan(
		&p.ID, &p.Slug, &p.Status, &brandID, &personalizationOptions,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.PersonalizationOptions, err = decodePersonalizationOptions(personalizationOptions)
	if err != nil {
		return nil, err
	}
	if brandID.Valid {
		var brand BrandView
		err := s.SQL.QueryRowContext(ctx, `
			SELECT id::text, slug, name
			FROM brands
			WHERE shop_id = $1 AND id = $2
		`, shopID, brandID.String).Scan(&brand.ID, &brand.Slug, &brand.Name)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("product references a brand outside its shop")
		}
		if err != nil {
			return nil, err
		}
		p.Brand = &brand
	}

	supplierRows, err := s.SQL.QueryContext(ctx, `
		SELECT product_supplier.supplier_id::text
		FROM product_suppliers product_supplier
		JOIN suppliers supplier
		  ON supplier.id = product_supplier.supplier_id AND supplier.shop_id = $2
		WHERE product_supplier.product_id = $1
		ORDER BY product_supplier.supplier_id
	`, productID, shopID)
	if err != nil {
		return nil, err
	}
	for supplierRows.Next() {
		var supplierID string
		if err := supplierRows.Scan(&supplierID); err != nil {
			_ = supplierRows.Close()
			return nil, err
		}
		p.SupplierIDs = append(p.SupplierIDs, supplierID)
	}
	if err := supplierRows.Err(); err != nil {
		_ = supplierRows.Close()
		return nil, err
	}
	if err := supplierRows.Close(); err != nil {
		return nil, err
	}

	trRows, err := s.SQL.QueryContext(ctx, `
		SELECT locale, name, description
		FROM product_translations
		WHERE product_id = $1
		ORDER BY locale
	`, productID)
	if err != nil {
		return nil, err
	}
	defer trRows.Close()
	for trRows.Next() {
		var t TranslationView
		if err := trRows.Scan(&t.Locale, &t.Name, &t.Description); err != nil {
			return nil, err
		}
		p.Translations = append(p.Translations, t)
	}
	if err := trRows.Err(); err != nil {
		return nil, err
	}

	categoryRows, err := s.SQL.QueryContext(ctx, `
		SELECT category_id::text
		FROM product_categories
		WHERE product_id = $1
		ORDER BY category_id
	`, productID)
	if err != nil {
		return nil, err
	}
	for categoryRows.Next() {
		var categoryID string
		if err := categoryRows.Scan(&categoryID); err != nil {
			_ = categoryRows.Close()
			return nil, err
		}
		p.CategoryIDs = append(p.CategoryIDs, categoryID)
	}
	if err := categoryRows.Err(); err != nil {
		_ = categoryRows.Close()
		return nil, err
	}
	if err := categoryRows.Close(); err != nil {
		return nil, err
	}

	mediaRows, err := s.SQL.QueryContext(ctx, `
		SELECT m.id::text, pm.position, m.uri, m.mime_type, m.alt
		FROM product_media pm
		JOIN media m ON m.id = pm.media_id
		WHERE pm.product_id = $1 AND m.shop_id = $2
		ORDER BY pm.position, m.id
	`, productID, shopID)
	if err != nil {
		return nil, err
	}
	defer mediaRows.Close()
	for mediaRows.Next() {
		var m MediaView
		if err := mediaRows.Scan(&m.ID, &m.Position, &m.URI, &m.MimeType, &m.Alt); err != nil {
			return nil, err
		}
		p.Media = append(p.Media, m)
	}
	if err := mediaRows.Err(); err != nil {
		return nil, err
	}

	vRows, err := s.SQL.QueryContext(ctx, `
		SELECT v.id::text, v.sku, v.options, v.weight_grams, COALESCE(i.quantity, 0), COALESCE(i.track, true)
		FROM variants v
		LEFT JOIN inventory_items i ON i.variant_id = v.id
		WHERE v.product_id = $1
		ORDER BY v.sku
	`, productID)
	if err != nil {
		return nil, err
	}
	defer vRows.Close()

	var variants []VariantView
	for vRows.Next() {
		var v VariantView
		var options []byte
		var weight sql.NullInt64
		if err := vRows.Scan(&v.ID, &v.SKU, &options, &weight, &v.Inventory.Quantity, &v.Inventory.Track); err != nil {
			return nil, err
		}
		if weight.Valid {
			v.WeightGrams = &weight.Int64
		}
		if err := unmarshalObject(options, &v.Options); err != nil {
			return nil, err
		}
		if v.Options == nil {
			v.Options = map[string]any{}
		}
		variants = append(variants, v)
	}
	if err := vRows.Err(); err != nil {
		return nil, err
	}

	for i := range variants {
		prices, err := s.pricesFor(ctx, variants[i].ID)
		if err != nil {
			return nil, err
		}
		variants[i].Prices = prices
		variants[i].Suppliers, err = s.variantSuppliersFor(ctx, shopID, variants[i].ID)
		if err != nil {
			return nil, err
		}
	}
	p.Variants = variants
	if p.Translations == nil {
		p.Translations = []TranslationView{}
	}
	if p.CategoryIDs == nil {
		p.CategoryIDs = []string{}
	}
	if p.Media == nil {
		p.Media = []MediaView{}
	}
	if p.Variants == nil {
		p.Variants = []VariantView{}
	}
	if p.SupplierIDs == nil {
		p.SupplierIDs = []string{}
	}
	return &p, nil
}

func (s *Store) variantSuppliersFor(ctx context.Context, shopID, variantID string) ([]VariantSupplierView, error) {
	rows, err := s.SQL.QueryContext(ctx, `
		SELECT supplier.id::text, supplier.name, variant_supplier.supplier_sku
		FROM variant_suppliers variant_supplier
		JOIN suppliers supplier ON supplier.id = variant_supplier.supplier_id
		JOIN variants variant ON variant.id = variant_supplier.variant_id
		JOIN products product ON product.id = variant.product_id
		WHERE variant_supplier.variant_id = $1 AND product.shop_id = $2 AND supplier.shop_id = $2
		ORDER BY supplier.code, supplier.id
	`, variantID, shopID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	suppliers := make([]VariantSupplierView, 0)
	for rows.Next() {
		var supplier VariantSupplierView
		if err := rows.Scan(&supplier.SupplierID, &supplier.SupplierName, &supplier.SupplierSKU); err != nil {
			return nil, err
		}
		suppliers = append(suppliers, supplier)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return suppliers, nil
}

func (s *Store) pricesFor(ctx context.Context, variantID string) ([]PriceView, error) {
	rows, err := s.SQL.QueryContext(ctx, `
		SELECT currency, amount::text, compare_at::text
		FROM prices
		WHERE variant_id = $1
		ORDER BY currency
	`, variantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PriceView
	for rows.Next() {
		var p PriceView
		var compare sql.NullString
		if err := rows.Scan(&p.Currency, &p.Amount, &compare); err != nil {
			return nil, err
		}
		if compare.Valid {
			p.CompareAt = &compare.String
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []PriceView{}
	}
	return out, nil
}
