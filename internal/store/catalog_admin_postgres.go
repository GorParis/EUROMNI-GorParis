package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
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
	err = tx.QueryRowContext(ctx, `
		INSERT INTO products (shop_id, slug, status)
		VALUES ($1, $2, $3)
		RETURNING id::text
	`, shopID, input.Slug, input.Status).Scan(&product.ID)
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
	if err := tx.Commit(); err != nil {
		return nil, catalogDBError(err)
	}
	product.Translations = append([]TranslationView(nil), input.Translations...)
	product.Media = []MediaView{}
	product.Variants = []VariantView{}
	return &product, nil
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
	err = tx.QueryRowContext(ctx, `
		INSERT INTO categories (shop_id, slug)
		VALUES ($1, $2)
		RETURNING id::text
	`, shopID, input.Slug).Scan(&category.ID)
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

func catalogDBError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return fmt.Errorf("%w: %v", ErrConflict, err)
	}
	return err
}
