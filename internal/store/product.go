package store

import (
	"context"
	"database/sql"
	"errors"
)

var ErrNotFound = errors.New("not found")

type ProductView struct {
	ID           string            `json:"id"`
	Slug         string            `json:"slug"`
	Status       string            `json:"status"`
	Translations []TranslationView `json:"translations"`
	CategoryIDs  []string          `json:"category_ids"`
	Media        []MediaView       `json:"media"`
	Variants     []VariantView     `json:"variants"`
}

type TranslationView struct {
	Locale      string `json:"locale"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type MediaView struct {
	URI      string `json:"uri"`
	MimeType string `json:"mime_type"`
	Alt      string `json:"alt"`
}

type VariantView struct {
	ID        string         `json:"id"`
	SKU       string         `json:"sku"`
	Options   map[string]any `json:"options"`
	Prices    []PriceView    `json:"prices"`
	Inventory InventoryView  `json:"inventory"`
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

var (
	_ ProductReader         = (*Store)(nil)
	_ ProductCategoryReader = (*Store)(nil)
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
	query := `
		SELECT id::text, slug, status::text
		FROM products
		WHERE shop_id = $1 AND id = $2
	`
	if activeOnly {
		query += ` AND status = 'active'`
	}
	err := s.SQL.QueryRowContext(ctx, query, shopID, productID).Scan(&p.ID, &p.Slug, &p.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
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
		SELECT m.uri, m.mime_type, m.alt
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
		if err := mediaRows.Scan(&m.URI, &m.MimeType, &m.Alt); err != nil {
			return nil, err
		}
		p.Media = append(p.Media, m)
	}
	if err := mediaRows.Err(); err != nil {
		return nil, err
	}

	vRows, err := s.SQL.QueryContext(ctx, `
		SELECT v.id::text, v.sku, v.options, COALESCE(i.quantity, 0), COALESCE(i.track, true)
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
		if err := vRows.Scan(&v.ID, &v.SKU, &options, &v.Inventory.Quantity, &v.Inventory.Track); err != nil {
			return nil, err
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
	return &p, nil
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
