package sqlitestore

import (
	"context"
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
	if err := tx.Commit(); err != nil {
		return nil, catalogSQLiteError(err)
	}
	return &store.ProductView{
		ID:           id,
		Slug:         input.Slug,
		Status:       input.Status,
		Translations: append([]store.TranslationView(nil), input.Translations...),
		Media:        []store.MediaView{},
		Variants:     []store.VariantView{},
	}, nil
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

func catalogSQLiteError(err error) error {
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) && (sqliteErr.Code() == 2067 || sqliteErr.Code() == 1555) {
		return fmt.Errorf("%w: %v", store.ErrConflict, err)
	}
	return err
}
