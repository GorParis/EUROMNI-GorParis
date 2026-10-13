package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	productCategories map[string][]string
	mu                sync.RWMutex
	carts             map[string]*memoryCart
}

type memoryCart struct {
	currency  string
	lines     map[string]int
	expiresAt time.Time
}

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
	return cloneProduct(p)
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
		clone, err := cloneProduct(product)
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
		clone, err := cloneProduct(product)
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
		lines:     make(map[string]int),
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
	lines := make(map[string]int, len(cart.lines))
	for variantID, quantity := range cart.lines {
		lines[variantID] = quantity
	}
	m.mu.RUnlock()

	items := make([]CartLineView, 0, len(lines))
	total := new(big.Rat)
	variantIDs := make([]string, 0, len(lines))
	for variantID := range lines {
		variantIDs = append(variantIDs, variantID)
	}
	sort.Strings(variantIDs)
	for _, variantID := range variantIDs {
		_, variant := m.findVariant(shopID, variantID)
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
		lineAmount := new(big.Rat).Mul(unitAmount, big.NewRat(int64(lines[variantID]), 1))
		total.Add(total, lineAmount)
		items = append(items, CartLineView{
			VariantID:  variant.ID,
			SKU:        variant.SKU,
			Quantity:   lines[variantID],
			UnitAmount: price.Amount,
		})
	}
	return &CartView{
		Items: items,
		Total: CartTotalView{Amount: total.FloatString(4), Currency: currency},
	}, nil
}

func (m *Memory) AddCartLine(ctx context.Context, shopID string, tokenHash []byte, variantID string, quantity int) (*CartView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if quantity < 1 || quantity > maxCartLineQuantity {
		return nil, ErrInvalidCartQuantity
	}
	if shopID != seed.ShopID || len(tokenHash) != sha256.Size {
		return nil, ErrInvalidCartToken
	}
	_, variant := m.findVariant(shopID, variantID)
	if variant == nil {
		return nil, ErrNotFound
	}
	m.mu.Lock()
	key := hex.EncodeToString(tokenHash)
	cart, ok := m.carts[key]
	if !ok || !time.Now().Before(cart.expiresAt) {
		m.mu.Unlock()
		return nil, ErrInvalidCartToken
	}
	currentQuantity := cart.lines[variantID]
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
	cart.lines[variantID] = currentQuantity + quantity
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
	return &out, nil
}
