package store

import (
	"context"
	"sort"
	"strings"
)

var _ CatalogReferenceWriter = (*Memory)(nil)

func (m *Memory) ListBrands(ctx context.Context, shopID string) ([]BrandView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	brands := make([]BrandView, 0)
	prefix := shopID + "/"
	for key, brand := range m.brands {
		if strings.HasPrefix(key, prefix) {
			brands = append(brands, *cloneBrand(brand))
		}
	}
	sortBrands(brands)
	return brands, nil
}

func (m *Memory) CreateBrand(ctx context.Context, shopID string, input BrandInput) (*BrandView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	id, err := NewCatalogID()
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, brand := range m.brands {
		if strings.HasPrefix(key, shopID+"/") && brand.Slug == input.Slug {
			return nil, ErrConflict
		}
	}
	brand := &BrandView{ID: id, Slug: input.Slug, Name: input.Name}
	m.brands[shopID+"/"+id] = brand
	return cloneBrand(brand), nil
}

func (m *Memory) UpdateBrand(ctx context.Context, shopID, brandID string, input BrandInput) (*BrandView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key := shopID + "/" + brandID
	brand, exists := m.brands[key]
	if !exists {
		return nil, ErrNotFound
	}
	for otherKey, other := range m.brands {
		if otherKey != key && strings.HasPrefix(otherKey, shopID+"/") && other.Slug == input.Slug {
			return nil, ErrConflict
		}
	}
	brand.Slug, brand.Name = input.Slug, input.Name
	return cloneBrand(brand), nil
}

func (m *Memory) ListSuppliers(ctx context.Context, shopID string) ([]SupplierView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	suppliers := make([]SupplierView, 0)
	prefix := shopID + "/"
	for key, supplier := range m.suppliers {
		if strings.HasPrefix(key, prefix) {
			suppliers = append(suppliers, *cloneSupplier(supplier))
		}
	}
	sortSuppliers(suppliers)
	return suppliers, nil
}

func (m *Memory) CreateSupplier(ctx context.Context, shopID string, input SupplierInput) (*SupplierView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	id, err := NewCatalogID()
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, supplier := range m.suppliers {
		if strings.HasPrefix(key, shopID+"/") && strings.EqualFold(supplier.Code, input.Code) {
			return nil, ErrConflict
		}
	}
	supplier := &SupplierView{ID: id, Code: input.Code, Name: input.Name, Active: input.Active}
	m.suppliers[shopID+"/"+id] = supplier
	return cloneSupplier(supplier), nil
}

func (m *Memory) UpdateSupplier(ctx context.Context, shopID, supplierID string, input SupplierInput) (*SupplierView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key := shopID + "/" + supplierID
	supplier, exists := m.suppliers[key]
	if !exists {
		return nil, ErrNotFound
	}
	if supplier.Active && !input.Active && m.memorySupplierInUse(shopID, supplierID) {
		return nil, ErrConflict
	}
	for otherKey, other := range m.suppliers {
		if otherKey != key && strings.HasPrefix(otherKey, shopID+"/") && strings.EqualFold(other.Code, input.Code) {
			return nil, ErrConflict
		}
	}
	supplier.Code, supplier.Name, supplier.Active = input.Code, input.Name, input.Active
	return cloneSupplier(supplier), nil
}

func (m *Memory) memorySupplierInUse(shopID, supplierID string) bool {
	prefix := shopID + "/"
	for key, product := range m.products {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		for _, id := range product.SupplierIDs {
			if id == supplierID {
				return true
			}
		}
		for _, variant := range product.Variants {
			for _, supplier := range variant.Suppliers {
				if supplier.SupplierID == supplierID {
					return true
				}
			}
		}
	}
	return false
}

func (m *Memory) validateMemoryCatalogReferences(shopID, brandID string, supplierIDs []string, variants []VariantInput) error {
	if brandID != "" {
		if _, exists := m.brands[shopID+"/"+brandID]; !exists {
			return ErrNotFound
		}
	}
	for _, supplierID := range supplierIDs {
		if err := m.validateMemorySupplier(shopID, supplierID); err != nil {
			return err
		}
	}
	for _, variant := range variants {
		for _, supplier := range variant.Suppliers {
			if err := m.validateMemorySupplier(shopID, supplier.SupplierID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Memory) validateMemorySupplier(shopID, supplierID string) error {
	supplier, exists := m.suppliers[shopID+"/"+supplierID]
	if !exists || !supplier.Active {
		return ErrNotFound
	}
	return nil
}

func (m *Memory) attachMemoryVariantSuppliers(shopID string, variants []VariantView, inputs []VariantInput) error {
	inputsBySKU := make(map[string][]VariantSupplierInput, len(inputs))
	for _, input := range inputs {
		inputsBySKU[input.SKU] = input.Suppliers
	}
	for i := range variants {
		variants[i].Suppliers = make([]VariantSupplierView, 0)
		for _, input := range inputsBySKU[variants[i].SKU] {
			supplier := m.suppliers[shopID+"/"+input.SupplierID]
			if supplier == nil || !supplier.Active {
				return ErrNotFound
			}
			variants[i].Suppliers = append(variants[i].Suppliers, VariantSupplierView{
				SupplierID: supplier.ID, SupplierName: supplier.Name, SupplierSKU: input.SupplierSKU,
			})
		}
	}
	return nil
}

func cloneBrand(brand *BrandView) *BrandView {
	if brand == nil {
		return nil
	}
	copy := *brand
	return &copy
}

func cloneSupplier(supplier *SupplierView) *SupplierView {
	if supplier == nil {
		return nil
	}
	copy := *supplier
	return &copy
}

func sortBrands(brands []BrandView) {
	sort.Slice(brands, func(i, j int) bool {
		if brands[i].Name == brands[j].Name {
			return brands[i].ID < brands[j].ID
		}
		return brands[i].Name < brands[j].Name
	})
}

func sortSuppliers(suppliers []SupplierView) {
	sort.Slice(suppliers, func(i, j int) bool {
		if suppliers[i].Name == suppliers[j].Name {
			return suppliers[i].ID < suppliers[j].ID
		}
		return suppliers[i].Name < suppliers[j].Name
	})
}
