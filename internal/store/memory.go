package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"

	"euromni/internal/seed"
)

type Memory struct {
	products          map[string]*ProductView
	categories        map[string]*CategoryView
	brands            map[string]*BrandView
	suppliers         map[string]*SupplierView
	productCategories map[string][]string
	mu                sync.RWMutex
	carts             map[string]*memoryCart
}

type memoryCart struct {
	currency  string
	lines     map[string]memoryCartLine
	expiresAt time.Time
}

type memoryCartLine struct {
	variantID        string
	quantity         int
	personalizations []PersonalizationSelection
}

var (
	_ ProductReader         = (*Memory)(nil)
	_ ProductCategoryReader = (*Memory)(nil)
	_ CatalogAdminWriter    = (*Memory)(nil)
)

func NewMemory() *Memory {
	compare := "110.0000"
	p := &ProductView{
		ID:     seed.ProductID,
		Slug:   "cabas-lin",
		Status: "active",
		Translations: []TranslationView{
			{Locale: "en-GB", Name: "Linen tote", Description: "Everyday tote, leather handles."},
			{Locale: "fr-FR", Name: "Cabas en lin", Description: "Cabas quotidien, anse cuir."},
		},
		Media: []MediaView{
			{
				URI:      "/storefront/images/cabas-lin.svg",
				MimeType: "image/svg+xml",
				Alt:      "Cabas en lin naturel",
			},
		},
		Variants: []VariantView{
			{
				ID:      seed.VariantID,
				SKU:     "CABAS-LIN-NAT",
				Options: map[string]any{"color": "naturel", "size": "unique"},
				Prices: []PriceView{{
					Currency:  "EUR",
					Amount:    "89.0000",
					CompareAt: &compare,
				}},
				Inventory: InventoryView{Quantity: 24, Track: true},
			},
		},
	}
	category := &CategoryView{
		ID:   seed.CategoryID,
		Slug: "sacs",
		Translations: []CategoryTranslationView{
			{Locale: "en-GB", Name: "Bags", Description: "Canvas and leather bags"},
			{Locale: "fr-FR", Name: "Sacs", Description: "Sacs en toile et cuir"},
		},
	}
	belt := &ProductView{
		ID:     seed.SecondProductID,
		Slug:   "ceinture-cuir",
		Status: "active",
		Translations: []TranslationView{
			{Locale: "en-GB", Name: "Leather belt", Description: "Vegetable-tanned leather, brass buckle."},
			{Locale: "fr-FR", Name: "Ceinture en cuir", Description: "Cuir tanné végétal, boucle laiton."},
		},
		Media: []MediaView{
			{
				URI:      "/storefront/images/ceinture-cuir.svg",
				MimeType: "image/svg+xml",
				Alt:      "Ceinture en cuir brun",
			},
		},
		Variants: []VariantView{
			{
				ID:      seed.SecondVariantID,
				SKU:     "CEINT-CUIR-BRN",
				Options: map[string]any{"color": "brun", "size": "85"},
				Prices: []PriceView{{
					Currency: "EUR",
					Amount:   "45.0000",
				}},
				Inventory: InventoryView{Quantity: 12, Track: true},
			},
			{
				ID:        "e0e00000-0001-4000-8000-000000000048",
				SKU:       "CEINT-CUIR-BRN-90",
				Options:   map[string]any{"color": "brun", "size": "90"},
				Prices:    []PriceView{{Currency: "EUR", Amount: "45.0000"}},
				Inventory: InventoryView{Quantity: 9, Track: true},
			},
			{
				ID:        "e0e00000-0001-4000-8000-000000000049",
				SKU:       "CEINT-CUIR-BRN-95",
				Options:   map[string]any{"color": "brun", "size": "95"},
				Prices:    []PriceView{{Currency: "EUR", Amount: "47.0000"}},
				Inventory: InventoryView{Quantity: 6, Track: true},
			},
		},
	}
	shirt := &ProductView{
		ID:     seed.ThirdProductID,
		Slug:   "chemise-lin",
		Status: "active",
		Media:  []MediaView{{URI: "/storefront/images/chemise-lin.svg", MimeType: "image/svg+xml", Alt: "Chemise en lin écru"}},
		Translations: []TranslationView{
			{Locale: "en-GB", Name: "Linen shirt", Description: "A relaxed-fit shirt in breathable European linen."},
			{Locale: "fr-FR", Name: "Chemise en lin", Description: "Une chemise ample en lin européen, légère et respirante."},
		},
		Variants: []VariantView{
			{ID: seed.ShirtVariantID, SKU: "CHEMISE-LIN-S", Options: map[string]any{"taille": "S"}, Prices: []PriceView{{Currency: "EUR", Amount: "79.0000"}}, Inventory: InventoryView{Quantity: 5, Track: true}},
			{ID: "e0e00000-0001-4000-8000-000000000052", SKU: "CHEMISE-LIN-M", Options: map[string]any{"taille": "M"}, Prices: []PriceView{{Currency: "EUR", Amount: "79.0000"}}, Inventory: InventoryView{Quantity: 8, Track: true}},
			{ID: "e0e00000-0001-4000-8000-000000000053", SKU: "CHEMISE-LIN-L", Options: map[string]any{"taille": "L"}, Prices: []PriceView{{Currency: "EUR", Amount: "82.0000"}}, Inventory: InventoryView{Quantity: 4, Track: true}},
		},
	}
	mug := &ProductView{
		ID:     seed.MugProductID,
		Slug:   "tasse-gres",
		Status: "active",
		Media:  []MediaView{{URI: "/storefront/images/tasse-gres.svg", MimeType: "image/svg+xml", Alt: "Tasse en grès émaillée"}},
		Translations: []TranslationView{
			{Locale: "en-GB", Name: "Stoneware cup", Description: "A hand-finished stoneware cup, made in small batches."},
			{Locale: "fr-FR", Name: "Tasse en grès", Description: "Une tasse en grès façonnée et émaillée à la main."},
		},
		Variants: []VariantView{
			{ID: seed.MugVariantID, SKU: "TASSE-GRES-SABLE", Options: map[string]any{}, Prices: []PriceView{{Currency: "EUR", Amount: "28.0000"}}, Inventory: InventoryView{Quantity: 14, Track: true}},
		},
	}
	candle := &ProductView{
		ID:     seed.CandleProductID,
		Slug:   "bougie-cedre",
		Status: "active",
		Media:  []MediaView{{URI: "/storefront/images/bougie-cedre.svg", MimeType: "image/svg+xml", Alt: "Bougie artisanale au cèdre"}},
		Translations: []TranslationView{
			{Locale: "en-GB", Name: "Cedar candle", Description: "A plant-wax candle with cedar and dried herbs."},
			{Locale: "fr-FR", Name: "Bougie au cèdre", Description: "Une bougie en cire végétale parfumée au cèdre et aux herbes sèches."},
		},
		Variants: []VariantView{
			{ID: seed.CandleVariantID, SKU: "BOUGIE-CEDRE-180", Options: map[string]any{}, Prices: []PriceView{{Currency: "EUR", Amount: "24.0000"}}, Inventory: InventoryView{Quantity: 18, Track: true}},
		},
	}
	return &Memory{
		products: map[string]*ProductView{
			seed.ShopID + "/" + seed.ProductID:       p,
			seed.ShopID + "/" + seed.SecondProductID: belt,
			seed.ShopID + "/" + seed.ThirdProductID:  shirt,
			seed.ShopID + "/" + seed.MugProductID:    mug,
			seed.ShopID + "/" + seed.CandleProductID: candle,
		},
		categories: map[string]*CategoryView{
			seed.ShopID + "/" + seed.CategoryID: category,
		},
		brands:    make(map[string]*BrandView),
		suppliers: make(map[string]*SupplierView),
		productCategories: map[string][]string{
			seed.ShopID + "/" + seed.CategoryID: {seed.ProductID},
		},
		carts: make(map[string]*memoryCart),
	}
}

func (m *Memory) GetProduct(_ context.Context, shopID, productID string) (*ProductView, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.products[shopID+"/"+productID]
	if !ok || p.Status != "active" {
		return nil, ErrNotFound
	}
	return m.cloneProductWithCategories(shopID, p)
}

func (m *Memory) ListProducts(ctx context.Context, shopID string) ([]ProductView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	prefix := shopID + "/"
	products := make([]ProductView, 0)
	for key, product := range m.products {
		if !strings.HasPrefix(key, prefix) || product.Status != "active" {
			continue
		}
		clone, err := m.cloneProductWithCategories(shopID, product)
		if err != nil {
			return nil, err
		}
		products = append(products, *clone)
	}
	sort.Slice(products, func(i, j int) bool {
		if products[i].Slug == products[j].Slug {
			return products[i].ID < products[j].ID
		}
		return products[i].Slug < products[j].Slug
	})
	return products, nil
}

func (m *Memory) ListProductsByCategory(ctx context.Context, shopID, categoryID string) ([]ProductView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	productIDs := m.productCategories[shopID+"/"+categoryID]
	products := make([]ProductView, 0, len(productIDs))
	for _, productID := range productIDs {
		product, ok := m.products[shopID+"/"+productID]
		if !ok || product.Status != "active" {
			continue
		}
		clone, err := m.cloneProductWithCategories(shopID, product)
		if err != nil {
			return nil, err
		}
		products = append(products, *clone)
	}
	sort.Slice(products, func(i, j int) bool {
		if products[i].Slug == products[j].Slug {
			return products[i].ID < products[j].ID
		}
		return products[i].Slug < products[j].Slug
	})
	return products, nil
}

func (m *Memory) ListCategories(ctx context.Context, shopID string) ([]CategoryView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	prefix := shopID + "/"
	categories := make([]CategoryView, 0)
	for key, category := range m.categories {
		if strings.HasPrefix(key, prefix) {
			categories = append(categories, cloneCategory(category))
		}
	}
	sort.Slice(categories, func(i, j int) bool {
		if categories[i].Slug == categories[j].Slug {
			return categories[i].ID < categories[j].ID
		}
		return categories[i].Slug < categories[j].Slug
	})
	return categories, nil
}

func (m *Memory) GetCategory(ctx context.Context, shopID, categoryID string) (*CategoryView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	category, ok := m.categories[shopID+"/"+categoryID]
	if !ok {
		return nil, ErrNotFound
	}
	copy := cloneCategory(category)
	return &copy, nil
}

func cloneCategory(category *CategoryView) CategoryView {
	copy := *category
	copy.Translations = append([]CategoryTranslationView(nil), category.Translations...)
	return copy
}

func (m *Memory) ListAdminProducts(ctx context.Context, shopID string) ([]ProductView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	prefix := shopID + "/"
	products := make([]ProductView, 0)
	for key, product := range m.products {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		clone, err := m.cloneProductWithCategories(shopID, product)
		if err != nil {
			return nil, err
		}
		products = append(products, *clone)
	}
	sort.Slice(products, func(i, j int) bool {
		if products[i].Slug == products[j].Slug {
			return products[i].ID < products[j].ID
		}
		return products[i].Slug < products[j].Slug
	})
	return products, nil
}

func (m *Memory) CreateProduct(ctx context.Context, shopID string, input ProductInput) (*ProductView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	id, err := NewCatalogID()
	if err != nil {
		return nil, err
	}
	variants, err := newMemoryVariants(input.Variants)
	if err != nil {
		return nil, err
	}
	media, err := updatedMemoryMedia(nil, input.Media)
	if err != nil {
		return nil, err
	}
	key := shopID + "/" + id
	product := &ProductView{
		ID:                     id,
		Slug:                   input.Slug,
		Status:                 input.Status,
		SupplierIDs:            append([]string(nil), input.SupplierIDs...),
		PersonalizationOptions: clonePersonalizationOptions(input.PersonalizationOptions),
		Translations:           append([]TranslationView(nil), input.Translations...),
		CategoryIDs:            append([]string(nil), input.CategoryIDs...),
		Media:                  media,
		Variants:               variants,
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for existingKey, existing := range m.products {
		if strings.HasPrefix(existingKey, shopID+"/") && existing.Slug == input.Slug {
			return nil, ErrConflict
		}
	}
	if err := m.validateMemoryCategories(shopID, input.CategoryIDs); err != nil {
		return nil, err
	}
	if err := m.validateMemoryCatalogReferences(shopID, input.BrandID, input.SupplierIDs, input.Variants); err != nil {
		return nil, err
	}
	if input.BrandID != "" {
		brand := m.brands[shopID+"/"+input.BrandID]
		product.Brand = cloneBrand(brand)
	}
	if err := m.attachMemoryVariantSuppliers(shopID, product.Variants, input.Variants); err != nil {
		return nil, err
	}
	m.products[key] = product
	m.setMemoryProductCategories(shopID, id, input.CategoryIDs)
	clone, err := m.cloneProductWithCategories(shopID, product)
	if err != nil {
		return nil, err
	}
	return clone, nil
}

func (m *Memory) UpdateProduct(ctx context.Context, shopID, productID string, input ProductInput) (*ProductView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key := shopID + "/" + productID
	product, ok := m.products[key]
	if !ok {
		return nil, ErrNotFound
	}
	for existingKey, existing := range m.products {
		if existingKey != key && strings.HasPrefix(existingKey, shopID+"/") && existing.Slug == input.Slug {
			return nil, ErrConflict
		}
	}
	variants, err := m.updatedMemoryVariants(product, input.Variants)
	if err != nil {
		return nil, err
	}
	media, err := updatedMemoryMedia(product.Media, input.Media)
	if err != nil {
		return nil, err
	}
	if err := m.validateMemoryCategories(shopID, input.CategoryIDs); err != nil {
		return nil, err
	}
	if err := m.validateMemoryCatalogReferences(shopID, input.BrandID, input.SupplierIDs, input.Variants); err != nil {
		return nil, err
	}
	product.Slug = input.Slug
	product.Status = input.Status
	product.Brand = cloneBrand(m.brands[shopID+"/"+input.BrandID])
	product.SupplierIDs = append([]string(nil), input.SupplierIDs...)
	product.PersonalizationOptions = clonePersonalizationOptions(input.PersonalizationOptions)
	product.Translations = append([]TranslationView(nil), input.Translations...)
	product.CategoryIDs = append([]string(nil), input.CategoryIDs...)
	product.Media = media
	product.Variants = variants
	if err := m.attachMemoryVariantSuppliers(shopID, product.Variants, input.Variants); err != nil {
		return nil, err
	}
	m.setMemoryProductCategories(shopID, productID, input.CategoryIDs)
	return cloneProduct(product)
}

func updatedMemoryMedia(existing []MediaView, inputs []MediaInput) ([]MediaView, error) {
	existingByURI := make(map[string]MediaView, len(existing))
	for _, media := range existing {
		existingByURI[media.URI] = media
	}
	media := make([]MediaView, 0, len(inputs))
	for position, input := range inputs {
		entry, exists := existingByURI[input.URI]
		if !exists || entry.ID == "" {
			id, err := NewCatalogID()
			if err != nil {
				return nil, err
			}
			entry.ID = id
		}
		entry.Position = position
		entry.URI = input.URI
		entry.MimeType = input.MimeType
		entry.Alt = input.Alt
		media = append(media, entry)
	}
	return media, nil
}

func (m *Memory) ArchiveProduct(ctx context.Context, shopID, productID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	product, ok := m.products[shopID+"/"+productID]
	if !ok {
		return ErrNotFound
	}
	product.Status = "archived"
	return nil
}

func (m *Memory) CreateCategory(ctx context.Context, shopID string, input CategoryInput) (*CategoryView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	id, err := NewCatalogID()
	if err != nil {
		return nil, err
	}
	category := &CategoryView{ID: id, Slug: input.Slug, Translations: append([]CategoryTranslationView(nil), input.Translations...)}
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, existing := range m.categories {
		if strings.HasPrefix(key, shopID+"/") && existing.Slug == input.Slug {
			return nil, ErrConflict
		}
	}
	m.categories[shopID+"/"+id] = category
	copy := cloneCategory(category)
	return &copy, nil
}

func (m *Memory) UpdateCategory(ctx context.Context, shopID, categoryID string, input CategoryInput) (*CategoryView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key := shopID + "/" + categoryID
	category, ok := m.categories[key]
	if !ok {
		return nil, ErrNotFound
	}
	for existingKey, existing := range m.categories {
		if existingKey != key && strings.HasPrefix(existingKey, shopID+"/") && existing.Slug == input.Slug {
			return nil, ErrConflict
		}
	}
	category.Slug = input.Slug
	category.Translations = append([]CategoryTranslationView(nil), input.Translations...)
	copy := cloneCategory(category)
	return &copy, nil
}

func (m *Memory) DeleteCategory(ctx context.Context, shopID, categoryID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key := shopID + "/" + categoryID
	if _, ok := m.categories[key]; !ok {
		return ErrNotFound
	}
	delete(m.categories, key)
	delete(m.productCategories, key)
	for productKey, product := range m.products {
		if !strings.HasPrefix(productKey, shopID+"/") {
			continue
		}
		filtered := product.CategoryIDs[:0]
		for _, id := range product.CategoryIDs {
			if id != categoryID {
				filtered = append(filtered, id)
			}
		}
		product.CategoryIDs = filtered
	}
	return nil
}

func (m *Memory) cloneProductWithCategories(shopID string, product *ProductView) (*ProductView, error) {
	copy, err := cloneProduct(product)
	if err != nil {
		return nil, err
	}
	copy.CategoryIDs = make([]string, 0)
	for key, productIDs := range m.productCategories {
		if !strings.HasPrefix(key, shopID+"/") {
			continue
		}
		for _, productID := range productIDs {
			if productID == product.ID {
				copy.CategoryIDs = append(copy.CategoryIDs, strings.TrimPrefix(key, shopID+"/"))
			}
		}
	}
	sort.Strings(copy.CategoryIDs)
	return copy, nil
}

func (m *Memory) validateMemoryCategories(shopID string, categoryIDs []string) error {
	for _, categoryID := range categoryIDs {
		if _, exists := m.categories[shopID+"/"+categoryID]; !exists {
			return ErrNotFound
		}
	}
	return nil
}

func (m *Memory) setMemoryProductCategories(shopID, productID string, categoryIDs []string) {
	for key, productIDs := range m.productCategories {
		if !strings.HasPrefix(key, shopID+"/") {
			continue
		}
		kept := productIDs[:0]
		for _, existingID := range productIDs {
			if existingID != productID {
				kept = append(kept, existingID)
			}
		}
		m.productCategories[key] = kept
	}
	for _, categoryID := range categoryIDs {
		key := shopID + "/" + categoryID
		m.productCategories[key] = append(m.productCategories[key], productID)
	}
}

func newMemoryVariants(inputs []VariantInput) ([]VariantView, error) {
	variants := make([]VariantView, 0, len(inputs))
	for _, input := range inputs {
		id, err := NewCatalogID()
		if err != nil {
			return nil, err
		}
		variants = append(variants, VariantView{
			ID:          id,
			SKU:         input.SKU,
			Options:     cloneObject(input.Options),
			WeightGrams: cloneInt64(input.WeightGrams),
			Prices:      clonePrices(input.Prices),
			Inventory:   input.Inventory,
		})
	}
	return variants, nil
}

func (m *Memory) updatedMemoryVariants(product *ProductView, inputs []VariantInput) ([]VariantView, error) {
	existing := make(map[string]VariantView, len(product.Variants))
	for _, variant := range product.Variants {
		existing[variant.SKU] = variant
	}
	inputSKUs := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		inputSKUs[input.SKU] = struct{}{}
	}
	for _, variant := range product.Variants {
		if _, keep := inputSKUs[variant.SKU]; keep {
			continue
		}
		for _, cart := range m.carts {
			for _, line := range cart.lines {
				if line.variantID == variant.ID {
					return nil, fmt.Errorf("%w: variant %s is referenced by a cart", ErrConflict, variant.SKU)
				}
			}
		}
	}
	variants := make([]VariantView, 0, len(inputs))
	for _, input := range inputs {
		variant, exists := existing[input.SKU]
		if !exists {
			id, err := NewCatalogID()
			if err != nil {
				return nil, err
			}
			variant.ID = id
		}
		variant.SKU = input.SKU
		variant.Options = cloneObject(input.Options)
		variant.WeightGrams = cloneInt64(input.WeightGrams)
		variant.Prices = clonePrices(input.Prices)
		variant.Inventory = input.Inventory
		variants = append(variants, variant)
	}
	return variants, nil
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneObject(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	copy := make(map[string]any, len(value))
	for key, item := range value {
		copy[key] = item
	}
	return copy
}

func clonePrices(prices []PriceView) []PriceView {
	copy := make([]PriceView, len(prices))
	for i, price := range prices {
		copy[i] = price
		if price.CompareAt != nil {
			value := *price.CompareAt
			copy[i].CompareAt = &value
		}
	}
	return copy
}

func (m *Memory) CreateCart(ctx context.Context, shopID string, tokenHash []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if shopID != seed.ShopID || len(tokenHash) != sha256.Size {
		return ErrInvalidCartToken
	}
	key := hex.EncodeToString(tokenHash)
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.carts[key]; exists {
		return errors.New("cart token collision")
	}
	m.carts[key] = &memoryCart{
		currency:  "EUR",
		lines:     make(map[string]memoryCartLine),
		expiresAt: time.Now().Add(30 * 24 * time.Hour),
	}
	return nil
}

func (m *Memory) GetCart(ctx context.Context, shopID string, tokenHash []byte) (*CartView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if shopID != seed.ShopID || len(tokenHash) != sha256.Size {
		return nil, ErrInvalidCartToken
	}
	key := hex.EncodeToString(tokenHash)
	m.mu.RLock()
	cart, ok := m.carts[key]
	if !ok || !time.Now().Before(cart.expiresAt) {
		m.mu.RUnlock()
		return nil, ErrInvalidCartToken
	}
	currency := cart.currency
	lines := make(map[string]memoryCartLine, len(cart.lines))
	for lineKey, line := range cart.lines {
		line.personalizations = append([]PersonalizationSelection(nil), line.personalizations...)
		lines[lineKey] = line
	}
	m.mu.RUnlock()

	items := make([]CartLineView, 0, len(lines))
	total := new(big.Rat)
	lineKeys := make([]string, 0, len(lines))
	for lineKey := range lines {
		lineKeys = append(lineKeys, lineKey)
	}
	sort.Strings(lineKeys)
	for _, lineKey := range lineKeys {
		line := lines[lineKey]
		_, variant := m.findVariant(shopID, line.variantID)
		if variant == nil {
			return nil, ErrNotFound
		}
		price, ok := priceForCurrency(variant, currency)
		if !ok {
			return nil, ErrNotFound
		}
		unitAmount, ok := new(big.Rat).SetString(price.Amount)
		if !ok {
			return nil, errors.New("invalid product price")
		}
		lineAmount := new(big.Rat).Mul(unitAmount, big.NewRat(int64(line.quantity), 1))
		total.Add(total, lineAmount)
		items = append(items, CartLineView{
			VariantID:        variant.ID,
			SKU:              variant.SKU,
			Quantity:         line.quantity,
			UnitAmount:       price.Amount,
			Personalizations: line.personalizations,
		})
	}
	return &CartView{
		Items: items,
		Total: CartTotalView{Amount: total.FloatString(4), Currency: currency},
	}, nil
}

func (m *Memory) AddCartLine(ctx context.Context, shopID string, tokenHash []byte, variantID string, quantity int, selectionsInput ...map[string]string) (*CartView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if quantity < 1 || quantity > maxCartLineQuantity {
		return nil, ErrInvalidCartQuantity
	}
	submitted := map[string]string{}
	if len(selectionsInput) > 0 && selectionsInput[0] != nil {
		submitted = selectionsInput[0]
	}
	if shopID != seed.ShopID || len(tokenHash) != sha256.Size {
		return nil, ErrInvalidCartToken
	}
	product, variant := m.findVariant(shopID, variantID)
	if variant == nil {
		return nil, ErrNotFound
	}
	selections, _, personalizationHash, err := ValidatePersonalizationSelections(product.PersonalizationOptions, submitted)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	key := hex.EncodeToString(tokenHash)
	cart, ok := m.carts[key]
	if !ok || !time.Now().Before(cart.expiresAt) {
		m.mu.Unlock()
		return nil, ErrInvalidCartToken
	}
	currentQuantity := 0
	for _, line := range cart.lines {
		if line.variantID == variantID {
			currentQuantity += line.quantity
		}
	}
	lineKey := variantID + "/" + personalizationHash
	existingLine := cart.lines[lineKey]
	if currentQuantity+quantity > maxCartLineQuantity {
		m.mu.Unlock()
		return nil, ErrInvalidCartQuantity
	}
	if variant.Inventory.Track && currentQuantity+quantity > variant.Inventory.Quantity {
		m.mu.Unlock()
		return nil, ErrOutOfStock
	}
	if _, ok := priceForCurrency(variant, cart.currency); !ok {
		m.mu.Unlock()
		return nil, ErrNotFound
	}
	existingLine.variantID = variantID
	existingLine.quantity += quantity
	existingLine.personalizations = selections
	cart.lines[lineKey] = existingLine
	m.mu.Unlock()
	return m.GetCart(ctx, shopID, tokenHash)
}

func (m *Memory) findVariant(shopID, variantID string) (*ProductView, *VariantView) {
	prefix := shopID + "/"
	for key, product := range m.products {
		if !strings.HasPrefix(key, prefix) || product.Status != "active" {
			continue
		}
		for i := range product.Variants {
			if product.Variants[i].ID == variantID {
				return product, &product.Variants[i]
			}
		}
	}
	return nil, nil
}

func priceForCurrency(variant *VariantView, currency string) (PriceView, bool) {
	for _, price := range variant.Prices {
		if price.Currency == currency {
			return price, true
		}
	}
	return PriceView{}, false
}

func cloneProduct(p *ProductView) (*ProductView, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	var out ProductView
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	out.SupplierIDs = append([]string(nil), p.SupplierIDs...)
	for i := range out.Variants {
		if i < len(p.Variants) {
			out.Variants[i].Suppliers = append([]VariantSupplierView(nil), p.Variants[i].Suppliers...)
		}
	}
	return &out, nil
}
