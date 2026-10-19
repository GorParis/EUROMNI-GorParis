package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var _ CatalogReferenceWriter = (*Store)(nil)

func (s *Store) ListBrands(ctx context.Context, shopID string) ([]BrandView, error) {
	rows, err := s.SQL.QueryContext(ctx, `
		SELECT id::text, slug, name
		FROM brands
		WHERE shop_id = $1
		ORDER BY name, id
	`, shopID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	brands := make([]BrandView, 0)
	for rows.Next() {
		var brand BrandView
		if err := rows.Scan(&brand.ID, &brand.Slug, &brand.Name); err != nil {
			return nil, err
		}
		brands = append(brands, brand)
	}
	return brands, rows.Err()
}

func (s *Store) CreateBrand(ctx context.Context, shopID string, input BrandInput) (*BrandView, error) {
	id, err := NewCatalogID()
	if err != nil {
		return nil, err
	}
	_, err = s.SQL.ExecContext(ctx, `
		INSERT INTO brands (id, shop_id, slug, name) VALUES ($1, $2, $3, $4)
	`, id, shopID, input.Slug, input.Name)
	if err != nil {
		return nil, catalogDBError(err)
	}
	return &BrandView{ID: id, Slug: input.Slug, Name: input.Name}, nil
}

func (s *Store) UpdateBrand(ctx context.Context, shopID, brandID string, input BrandInput) (*BrandView, error) {
	result, err := s.SQL.ExecContext(ctx, `
		UPDATE brands SET slug = $3, name = $4, updated_at = now()
		WHERE shop_id = $1 AND id = $2
	`, shopID, brandID, input.Slug, input.Name)
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
	return &BrandView{ID: brandID, Slug: input.Slug, Name: input.Name}, nil
}

func (s *Store) ListSuppliers(ctx context.Context, shopID string) ([]SupplierView, error) {
	rows, err := s.SQL.QueryContext(ctx, `
		SELECT id::text, code, name, active
		FROM suppliers
		WHERE shop_id = $1
		ORDER BY name, id
	`, shopID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	suppliers := make([]SupplierView, 0)
	for rows.Next() {
		var supplier SupplierView
		if err := rows.Scan(&supplier.ID, &supplier.Code, &supplier.Name, &supplier.Active); err != nil {
			return nil, err
		}
		suppliers = append(suppliers, supplier)
	}
	return suppliers, rows.Err()
}

func (s *Store) CreateSupplier(ctx context.Context, shopID string, input SupplierInput) (*SupplierView, error) {
	id, err := NewCatalogID()
	if err != nil {
		return nil, err
	}
	_, err = s.SQL.ExecContext(ctx, `
		INSERT INTO suppliers (id, shop_id, code, name, active) VALUES ($1, $2, $3, $4, $5)
	`, id, shopID, input.Code, input.Name, input.Active)
	if err != nil {
		return nil, catalogDBError(err)
	}
	return &SupplierView{ID: id, Code: input.Code, Name: input.Name, Active: input.Active}, nil
}

func (s *Store) UpdateSupplier(ctx context.Context, shopID, supplierID string, input SupplierInput) (*SupplierView, error) {
	tx, err := s.SQL.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var existing bool
	err = tx.QueryRowContext(ctx, `
		SELECT active FROM suppliers WHERE shop_id = $1 AND id = $2 FOR UPDATE
	`, shopID, supplierID).Scan(&existing)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if existing && !input.Active {
		var references int
		if err := tx.QueryRowContext(ctx, `
			SELECT (SELECT count(*) FROM product_suppliers WHERE supplier_id = $1) +
			       (SELECT count(*) FROM variant_suppliers WHERE supplier_id = $1)
		`, supplierID).Scan(&references); err != nil {
			return nil, err
		}
		if references > 0 {
			return nil, fmt.Errorf("%w: supplier is assigned to catalog items", ErrConflict)
		}
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE suppliers SET code = $3, name = $4, active = $5, updated_at = now()
		WHERE shop_id = $1 AND id = $2
	`, shopID, supplierID, input.Code, input.Name, input.Active)
	if err != nil {
		return nil, catalogDBError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, catalogDBError(err)
	}
	return &SupplierView{ID: supplierID, Code: input.Code, Name: input.Name, Active: input.Active}, nil
}
