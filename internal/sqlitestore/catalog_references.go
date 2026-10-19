package sqlitestore

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"euromni/internal/store"
)

var _ store.CatalogReferenceWriter = (*Store)(nil)

func (s *Store) ListBrands(ctx context.Context, shopID string) ([]store.BrandView, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, slug, name FROM brands WHERE shop_id = ? ORDER BY name, id
	`, shopID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	brands := make([]store.BrandView, 0)
	for rows.Next() {
		var brand store.BrandView
		if err := rows.Scan(&brand.ID, &brand.Slug, &brand.Name); err != nil {
			return nil, err
		}
		brands = append(brands, brand)
	}
	return brands, rows.Err()
}

func (s *Store) CreateBrand(ctx context.Context, shopID string, input store.BrandInput) (*store.BrandView, error) {
	id, err := store.NewCatalogID()
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO brands (id, shop_id, slug, name, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, id, shopID, input.Slug, input.Name, now, now); err != nil {
		return nil, catalogSQLiteError(err)
	}
	return &store.BrandView{ID: id, Slug: input.Slug, Name: input.Name}, nil
}

func (s *Store) UpdateBrand(ctx context.Context, shopID, brandID string, input store.BrandInput) (*store.BrandView, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE brands SET slug = ?, name = ?, updated_at = ?
		WHERE shop_id = ? AND id = ?
	`, input.Slug, input.Name, time.Now().Unix(), shopID, brandID)
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
	return &store.BrandView{ID: brandID, Slug: input.Slug, Name: input.Name}, nil
}

func (s *Store) ListSuppliers(ctx context.Context, shopID string) ([]store.SupplierView, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, code, name, active FROM suppliers WHERE shop_id = ? ORDER BY name, id
	`, shopID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	suppliers := make([]store.SupplierView, 0)
	for rows.Next() {
		var supplier store.SupplierView
		if err := rows.Scan(&supplier.ID, &supplier.Code, &supplier.Name, &supplier.Active); err != nil {
			return nil, err
		}
		suppliers = append(suppliers, supplier)
	}
	return suppliers, rows.Err()
}

func (s *Store) CreateSupplier(ctx context.Context, shopID string, input store.SupplierInput) (*store.SupplierView, error) {
	id, err := store.NewCatalogID()
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO suppliers (id, shop_id, code, name, active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, id, shopID, input.Code, input.Name, input.Active, now, now); err != nil {
		return nil, catalogSQLiteError(err)
	}
	return &store.SupplierView{ID: id, Code: input.Code, Name: input.Name, Active: input.Active}, nil
}

func (s *Store) UpdateSupplier(ctx context.Context, shopID, supplierID string, input store.SupplierInput) (*store.SupplierView, error) {
	tx, err := s.beginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var existing bool
	err = tx.QueryRowContext(ctx, `
		SELECT active FROM suppliers WHERE shop_id = ? AND id = ?
	`, shopID, supplierID).Scan(&existing)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if existing && !input.Active {
		var references int
		if err := tx.QueryRowContext(ctx, `
			SELECT (SELECT count(*) FROM product_suppliers WHERE supplier_id = ?) +
			       (SELECT count(*) FROM variant_suppliers WHERE supplier_id = ?)
		`, supplierID, supplierID).Scan(&references); err != nil {
			return nil, err
		}
		if references > 0 {
			return nil, store.ErrConflict
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE suppliers SET code = ?, name = ?, active = ?, updated_at = ?
		WHERE shop_id = ? AND id = ?
	`, input.Code, input.Name, input.Active, time.Now().Unix(), shopID, supplierID); err != nil {
		return nil, catalogSQLiteError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, catalogSQLiteError(err)
	}
	return &store.SupplierView{ID: supplierID, Code: input.Code, Name: input.Name, Active: input.Active}, nil
}
