package sqlitestore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"euromni/internal/store"
	"modernc.org/sqlite"
)

var _ store.CatalogAdminWriter = (*Store)(nil)

func (s *Store) ListAdminProducts(ctx context.Context, shopID string) ([]store.ProductView, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id FROM products
		WHERE shop_id = ?
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
	products := make([]store.ProductView, 0, len(productIDs))
	for _, productID := range productIDs {
		product, err := s.getProduct(ctx, shopID, productID, false)
		if err != nil {
			return nil, err
		}
		products = append(products, *product)
	}
	return products, nil
}

func (s *Store) CreateProduct(ctx context.Context, shopID string, input store.ProductInput) (*store.ProductView, error) {
	id, err := store.NewCatalogID()
	if err != nil {
		return nil, err
	}
	tx, err := s.beginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO products (id, shop_id, slug, status) VALUES (?, ?, ?, ?)
	`, id, shopID, input.Slug, input.Status); err != nil {
		return nil, catalogSQLiteError(err)
	}
	for _, translation := range input.Translations {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO product_translations (product_id, locale, name, description)
			VALUES (?, ?, ?, ?)
		`, id, translation.Locale, translation.Name, translation.Description); err != nil {
			return nil, catalogSQLiteError(err)
		}
	}
	if err := replaceProductCategories(ctx, tx, shopID, id, input.CategoryIDs); err != nil {
		return nil, err
	}
	if err := replaceProductMedia(ctx, tx, shopID, id, input.Media); err != nil {
		return nil, err
	}
	if err := replaceProductVariants(ctx, tx, id, input.Variants); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, catalogSQLiteError(err)
	}
	return s.readAdminProduct(ctx, shopID, id)
}

func (s *Store) UpdateProduct(ctx context.Context, shopID, productID string, input store.ProductInput) (*store.ProductView, error) {
	tx, err := s.beginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		UPDATE products SET slug = ?, status = ? WHERE shop_id = ? AND id = ?
	`, input.Slug, input.Status, shopID, productID)
	if err != nil {
		return nil, catalogSQLiteError(err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, store.ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM product_translations WHERE product_id = ?`, productID); err != nil {
		return nil, err
	}
	for _, translation := range input.Translations {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO product_translations (product_id, locale, name, description)
			VALUES (?, ?, ?, ?)
		`, productID, translation.Locale, translation.Name, translation.Description); err != nil {
			return nil, catalogSQLiteError(err)
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
		return nil, catalogSQLiteError(err)
	}
	return s.readAdminProduct(ctx, shopID, productID)
}

func (s *Store) ArchiveProduct(ctx context.Context, shopID, productID string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE products SET status = 'archived' WHERE shop_id = ? AND id = ?
	`, shopID, productID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) CreateCategory(ctx context.Context, shopID string, input store.CategoryInput) (*store.CategoryView, error) {
	id, err := store.NewCatalogID()
	if err != nil {
		return nil, err
	}
	tx, err := s.beginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO categories (id, shop_id, slug) VALUES (?, ?, ?)`, id, shopID, input.Slug); err != nil {
		return nil, catalogSQLiteError(err)
	}
	for _, translation := range input.Translations {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO category_translations (category_id, locale, name, description)
			VALUES (?, ?, ?, ?)
		`, id, translation.Locale, translation.Name, translation.Description); err != nil {
			return nil, catalogSQLiteError(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, catalogSQLiteError(err)
	}
	return &store.CategoryView{ID: id, Slug: input.Slug, Translations: append([]store.CategoryTranslationView(nil), input.Translations...)}, nil
}

func (s *Store) UpdateCategory(ctx context.Context, shopID, categoryID string, input store.CategoryInput) (*store.CategoryView, error) {
	tx, err := s.beginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE categories SET slug = ? WHERE shop_id = ? AND id = ?`, input.Slug, shopID, categoryID)
	if err != nil {
		return nil, catalogSQLiteError(err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, store.ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM category_translations WHERE category_id = ?`, categoryID); err != nil {
		return nil, err
	}
	for _, translation := range input.Translations {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO category_translations (category_id, locale, name, description)
			VALUES (?, ?, ?, ?)
		`, categoryID, translation.Locale, translation.Name, translation.Description); err != nil {
			return nil, catalogSQLiteError(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, catalogSQLiteError(err)
	}
	return s.GetCategory(ctx, shopID, categoryID)
}

func (s *Store) DeleteCategory(ctx context.Context, shopID, categoryID string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM categories WHERE shop_id = ? AND id = ?`, shopID, categoryID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) readAdminProduct(ctx context.Context, shopID, productID string) (*store.ProductView, error) {
	return s.getProduct(ctx, shopID, productID, false)
}

func replaceProductCategories(ctx context.Context, tx *sql.Tx, shopID, productID string, categoryIDs []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM product_categories WHERE product_id = ?`, productID); err != nil {
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
			SELECT ?, id FROM categories WHERE id = ? AND shop_id = ?
		`, productID, categoryID, shopID)
		if err != nil {
			return catalogSQLiteError(err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected != 1 {
			return store.ErrNotFound
		}
	}
	return nil
}

func replaceProductMedia(ctx context.Context, tx *sql.Tx, shopID, productID string, inputs []store.MediaInput) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT m.id, m.uri
		FROM product_media pm
		JOIN media m ON m.id = pm.media_id
		WHERE pm.product_id = ? AND m.shop_id = ?
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
	if _, err := tx.ExecContext(ctx, `DELETE FROM product_media WHERE product_id = ?`, productID); err != nil {
		return err
	}
	for position, input := range inputs {
		mediaID, found := existing[input.URI]
		if found {
			if _, err := tx.ExecContext(ctx, `
				UPDATE media SET mime_type = ?, alt = ?
				WHERE id = ? AND shop_id = ?
			`, input.MimeType, input.Alt, mediaID, shopID); err != nil {
				return err
			}
		} else {
			mediaID, err = store.NewCatalogID()
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO media (id, shop_id, uri, mime_type, alt)
				VALUES (?, ?, ?, ?, ?)
			`, mediaID, shopID, input.URI, input.MimeType, input.Alt); err != nil {
				return catalogSQLiteError(err)
			}
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO product_media (product_id, media_id, position)
			VALUES (?, ?, ?)
		`, productID, mediaID, position); err != nil {
			return catalogSQLiteError(err)
		}
	}
	return nil
}

func replaceProductVariants(ctx context.Context, tx *sql.Tx, productID string, inputs []store.VariantInput) error {
	rows, err := tx.QueryContext(ctx, `SELECT id, sku FROM variants WHERE product_id = ?`, productID)
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
			variantID, err = store.NewCatalogID()
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO variants (id, product_id, sku, options) VALUES (?, ?, ?, ?)
			`, variantID, productID, input.SKU, string(options)); err != nil {
				return catalogSQLiteError(err)
			}
		} else if _, err := tx.ExecContext(ctx, `UPDATE variants SET options = ? WHERE id = ?`, string(options), variantID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO inventory_items (variant_id, quantity, track) VALUES (?, ?, ?)
			ON CONFLICT (variant_id) DO UPDATE SET quantity = excluded.quantity, track = excluded.track
		`, variantID, input.Inventory.Quantity, input.Inventory.Track); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM prices WHERE variant_id = ?`, variantID); err != nil {
			return err
		}
		for _, price := range input.Prices {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO prices (variant_id, currency, amount, compare_at) VALUES (?, ?, ?, ?)
			`, variantID, price.Currency, price.Amount, price.CompareAt); err != nil {
				return catalogSQLiteError(err)
			}
		}
	}
	for sku, variantID := range existing {
		if _, ok := kept[sku]; ok {
			continue
		}
		var references int
		if err := tx.QueryRowContext(ctx, `
			SELECT (SELECT count(*) FROM cart_lines WHERE variant_id = ?) +
			       (SELECT count(*) FROM order_lines WHERE variant_id = ?)
		`, variantID, variantID).Scan(&references); err != nil {
			return err
		}
		if references > 0 {
			return fmt.Errorf("%w: variant %s is referenced by an order or cart", store.ErrConflict, sku)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM variants WHERE id = ?`, variantID); err != nil {
			return err
		}
	}
	return nil
}

func catalogSQLiteError(err error) error {
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) && (sqliteErr.Code() == 2067 || sqliteErr.Code() == 1555) {
		return fmt.Errorf("%w: %v", store.ErrConflict, err)
	}
	return err
}
