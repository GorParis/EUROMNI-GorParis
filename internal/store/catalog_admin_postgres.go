package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"euromni/internal/dbdialect"
)

var _ CatalogAdminWriter = (*Store)(nil)

func (s *Store) ListAdminProducts(ctx context.Context, shopID string) ([]ProductView, error) {
	if s == nil || s.SQL == nil {
		return nil, errors.New("store not configured")
	}
	rows, err := s.SQL.QueryContext(ctx, `
		SELECT id::text
		FROM products
		WHERE shop_id = $1
		ORDER BY slug, id
	`, shopID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	productIDs := make([]string, 0)
	for rows.Next() {
		var productID string
		if err := rows.Scan(&productID); err != nil {
			return nil, err
		}
		productIDs = append(productIDs, productID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	products := make([]ProductView, 0, len(productIDs))
	for _, productID := range productIDs {
		product, err := s.getProduct(ctx, shopID, productID, false)
		if err != nil {
			return nil, err
		}
		products = append(products, *product)
	}
	return products, nil
}

func (s *Store) CreateProduct(ctx context.Context, shopID string, input ProductInput) (*ProductView, error) {
	if s == nil || s.SQL == nil {
		return nil, errors.New("store not configured")
	}
	tx, err := s.SQL.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var product ProductView
	product.ID, err = NewCatalogID()
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO products (id, shop_id, slug, status)
		VALUES ($1, $2, $3, $4)
	`, product.ID, shopID, input.Slug, input.Status)
	if err != nil {
		return nil, catalogDBError(err)
	}
	product.Slug, product.Status = input.Slug, input.Status
	for _, translation := range input.Translations {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO product_translations (product_id, locale, name, description)
			VALUES ($1, $2, $3, $4)
		`, product.ID, translation.Locale, translation.Name, translation.Description); err != nil {
			return nil, catalogDBError(err)
		}
	}
	if err := replaceProductCategories(ctx, tx, shopID, product.ID, input.CategoryIDs); err != nil {
		return nil, err
	}
	if err := replaceProductMedia(ctx, tx, shopID, product.ID, input.Media); err != nil {
		return nil, err
	}
	if err := replaceProductVariants(ctx, tx, product.ID, input.Variants); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, catalogDBError(err)
	}
	return s.readAdminProduct(ctx, shopID, product.ID)
}

func (s *Store) UpdateProduct(ctx context.Context, shopID, productID string, input ProductInput) (*ProductView, error) {
	if s == nil || s.SQL == nil {
		return nil, errors.New("store not configured")
	}
	tx, err := s.SQL.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		UPDATE products
		SET slug = $3, status = $4, updated_at = now()
		WHERE shop_id = $1 AND id = $2
	`, shopID, productID, input.Slug, input.Status)
	if err != nil {
		return nil, catalogDBError(err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM product_translations WHERE product_id = $1`, productID); err != nil {
		return nil, err
	}
	for _, translation := range input.Translations {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO product_translations (product_id, locale, name, description)
			VALUES ($1, $2, $3, $4)
		`, productID, translation.Locale, translation.Name, translation.Description); err != nil {
			return nil, catalogDBError(err)
		}
	}
	if err := replaceProductCategories(ctx, tx, shopID, productID, input.CategoryIDs); err != nil {
		return nil, err
	}
	if err := replaceProductMedia(ctx, tx, shopID, productID, input.Media); err != nil {
		return nil, err
	}
	if err := replaceProductVariants(ctx, tx, productID, input.Variants); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, catalogDBError(err)
	}
	return s.readAdminProduct(ctx, shopID, productID)
}

func (s *Store) ArchiveProduct(ctx context.Context, shopID, productID string) error {
	if s == nil || s.SQL == nil {
		return errors.New("store not configured")
	}
	result, err := s.SQL.ExecContext(ctx, `
		UPDATE products SET status = 'archived', updated_at = now()
		WHERE shop_id = $1 AND id = $2
	`, shopID, productID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CreateCategory(ctx context.Context, shopID string, input CategoryInput) (*CategoryView, error) {
	if s == nil || s.SQL == nil {
		return nil, errors.New("store not configured")
	}
	tx, err := s.SQL.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var category CategoryView
	category.ID, err = NewCatalogID()
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO categories (id, shop_id, slug)
		VALUES ($1, $2, $3)
	`, category.ID, shopID, input.Slug)
	if err != nil {
		return nil, catalogDBError(err)
	}
	category.Slug = input.Slug
	for _, translation := range input.Translations {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO category_translations (category_id, locale, name, description)
			VALUES ($1, $2, $3, $4)
		`, category.ID, translation.Locale, translation.Name, translation.Description); err != nil {
			return nil, catalogDBError(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, catalogDBError(err)
	}
	category.Translations = append([]CategoryTranslationView(nil), input.Translations...)
	return &category, nil
}

func (s *Store) UpdateCategory(ctx context.Context, shopID, categoryID string, input CategoryInput) (*CategoryView, error) {
	if s == nil || s.SQL == nil {
		return nil, errors.New("store not configured")
	}
	tx, err := s.SQL.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		UPDATE categories SET slug = $3, updated_at = now()
		WHERE shop_id = $1 AND id = $2
	`, shopID, categoryID, input.Slug)
	if err != nil {
		return nil, catalogDBError(err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM category_translations WHERE category_id = $1`, categoryID); err != nil {
		return nil, err
	}
	for _, translation := range input.Translations {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO category_translations (category_id, locale, name, description)
			VALUES ($1, $2, $3, $4)
		`, categoryID, translation.Locale, translation.Name, translation.Description); err != nil {
			return nil, catalogDBError(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, catalogDBError(err)
	}
	return s.GetCategory(ctx, shopID, categoryID)
}

func (s *Store) DeleteCategory(ctx context.Context, shopID, categoryID string) error {
	if s == nil || s.SQL == nil {
		return errors.New("store not configured")
	}
	result, err := s.SQL.ExecContext(ctx, `DELETE FROM categories WHERE shop_id = $1 AND id = $2`, shopID, categoryID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) readAdminProduct(ctx context.Context, shopID, productID string) (*ProductView, error) {
	return s.getProduct(ctx, shopID, productID, false)
}

func replaceProductCategories(ctx context.Context, tx *sql.Tx, shopID, productID string, categoryIDs []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM product_categories WHERE product_id = $1`, productID); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(categoryIDs))
	for _, categoryID := range categoryIDs {
		if _, exists := seen[categoryID]; exists {
			continue
		}
		seen[categoryID] = struct{}{}
		result, err := tx.ExecContext(ctx, `
			INSERT INTO product_categories (product_id, category_id)
			SELECT $1, id FROM categories WHERE id = $2 AND shop_id = $3
		`, productID, categoryID, shopID)
		if err != nil {
			return catalogDBError(err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected != 1 {
			return ErrNotFound
		}
	}
	return nil
}

func replaceProductMedia(ctx context.Context, tx *sql.Tx, shopID, productID string, inputs []MediaInput) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT m.id::text, m.uri
		FROM product_media pm
		JOIN media m ON m.id = pm.media_id
		WHERE pm.product_id = $1 AND m.shop_id = $2
		FOR UPDATE OF m
	`, productID, shopID)
	if err != nil {
		return err
	}
	existing := make(map[string]string)
	for rows.Next() {
		var id, uri string
		if err := rows.Scan(&id, &uri); err != nil {
			_ = rows.Close()
			return err
		}
		existing[uri] = id
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM product_media WHERE product_id = $1`, productID); err != nil {
		return err
	}
	for position, input := range inputs {
		mediaID, found := existing[input.URI]
		if found {
			if _, err := tx.ExecContext(ctx, `
				UPDATE media SET mime_type = $2, alt = $3
				WHERE id = $1 AND shop_id = $4
			`, mediaID, input.MimeType, input.Alt, shopID); err != nil {
				return err
			}
		} else {
			mediaID, err = NewCatalogID()
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO media (id, shop_id, uri, mime_type, alt)
				VALUES ($1, $2, $3, $4, $5)
			`, mediaID, shopID, input.URI, input.MimeType, input.Alt); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO product_media (product_id, media_id, position)
			VALUES ($1, $2, $3)
		`, productID, mediaID, position); err != nil {
			return err
		}
	}
	return nil
}

func replaceProductVariants(ctx context.Context, tx *sql.Tx, productID string, inputs []VariantInput) error {
	rows, err := tx.QueryContext(ctx, `SELECT id::text, sku FROM variants WHERE product_id = $1 FOR UPDATE`, productID)
	if err != nil {
		return err
	}
	existing := make(map[string]string)
	for rows.Next() {
		var id, sku string
		if err := rows.Scan(&id, &sku); err != nil {
			_ = rows.Close()
			return err
		}
		existing[sku] = id
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	kept := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		kept[input.SKU] = struct{}{}
		variantID, found := existing[input.SKU]
		options, err := json.Marshal(input.Options)
		if err != nil {
			return fmt.Errorf("encode variant options: %w", err)
		}
		if !found {
			variantID, err = NewCatalogID()
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO variants (id, product_id, sku, options)
				VALUES ($1, $2, $3, $4)
			`, variantID, productID, input.SKU, string(options)); err != nil {
				return catalogDBError(err)
			}
		} else if _, err := tx.ExecContext(ctx, `
			UPDATE variants SET options = $2, updated_at = now() WHERE id = $1
		`, variantID, string(options)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO inventory_items (variant_id, quantity, track)
			VALUES ($1, $2, $3)
			ON CONFLICT (variant_id) DO UPDATE SET quantity = EXCLUDED.quantity,
				track = EXCLUDED.track, updated_at = now()
		`, variantID, input.Inventory.Quantity, input.Inventory.Track); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM prices WHERE variant_id = $1`, variantID); err != nil {
			return err
		}
		for _, price := range input.Prices {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO prices (variant_id, currency, amount, compare_at)
				VALUES ($1, $2, $3, $4)
			`, variantID, price.Currency, price.Amount, price.CompareAt); err != nil {
				return catalogDBError(err)
			}
		}
	}
	for sku, variantID := range existing {
		if _, ok := kept[sku]; ok {
			continue
		}
		var references int
		if err := tx.QueryRowContext(ctx, `
			SELECT (SELECT count(*) FROM cart_lines WHERE variant_id = $1) +
			       (SELECT count(*) FROM order_lines WHERE variant_id = $1)
		`, variantID).Scan(&references); err != nil {
			return err
		}
		if references > 0 {
			return fmt.Errorf("%w: variant %s is referenced by an order or cart", ErrConflict, sku)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM variants WHERE id = $1`, variantID); err != nil {
			return err
		}
	}
	return nil
}

func catalogDBError(err error) error {
	if dbdialect.IsUniqueConstraint(err) {
		return fmt.Errorf("%w: %v", ErrConflict, err)
	}
	return err
}
