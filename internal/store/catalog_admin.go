package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
)

var ErrConflict = errors.New("catalog conflict")

type ProductInput struct {
	Slug                   string                  `json:"slug"`
	Status                 string                  `json:"status"`
	Volume                 *ProductVolume          `json:"volume,omitempty"`
	Tags                   []string                `json:"tags"`
	BrandID                string                  `json:"brand_id,omitempty"`
	SupplierIDs            []string                `json:"supplier_ids"`
	PersonalizationOptions []PersonalizationOption `json:"personalization_options"`
	Translations           []TranslationView       `json:"translations"`
	CategoryIDs            []string                `json:"category_ids"`
	Media                  []MediaInput            `json:"media"`
	Variants               []VariantInput          `json:"variants"`
}

type MediaInput struct {
	URI      string `json:"uri"`
	MimeType string `json:"mime_type"`
	Alt      string `json:"alt"`
}

type VariantInput struct {
	SKU         string                 `json:"sku"`
	Options     map[string]any         `json:"options"`
	WeightGrams *int64                 `json:"weight_grams,omitempty"`
	Prices      []PriceView            `json:"prices"`
	Inventory   InventoryView          `json:"inventory"`
	Suppliers   []VariantSupplierInput `json:"suppliers"`
}

type VariantSupplierInput struct {
	SupplierID  string `json:"supplier_id"`
	SupplierSKU string `json:"supplier_sku"`
}

type CategoryInput struct {
	Slug         string                    `json:"slug"`
	Translations []CategoryTranslationView `json:"translations"`
}

type CatalogAdminWriter interface {
	ListAdminProducts(ctx context.Context, shopID string) ([]ProductView, error)
	CreateProduct(ctx context.Context, shopID string, input ProductInput) (*ProductView, error)
	UpdateProduct(ctx context.Context, shopID, productID string, input ProductInput) (*ProductView, error)
	ArchiveProduct(ctx context.Context, shopID, productID string) error
	CreateCategory(ctx context.Context, shopID string, input CategoryInput) (*CategoryView, error)
	UpdateCategory(ctx context.Context, shopID, categoryID string, input CategoryInput) (*CategoryView, error)
	DeleteCategory(ctx context.Context, shopID, categoryID string) error
}

type SupplierProductDraftInput struct {
	Provider          string                   `json:"provider"`
	ExternalID        string                   `json:"external_id"`
	PublicationStatus string                   `json:"publication_status"`
	Name              string                   `json:"name"`
	Description       string                   `json:"description"`
	Locale            *string                  `json:"locale"`
	Price             SupplierProductPrice     `json:"price"`
	Inventory         SupplierProductInventory `json:"inventory"`
	Weight            SupplierProductWeight    `json:"weight"`
	Volume            SupplierProductVolume    `json:"volume"`
	Tags              []string                 `json:"tags"`
	Unresolved        []string                 `json:"unresolved"`
}

type SupplierProductPrice struct {
	Amount   string  `json:"amount"`
	Currency *string `json:"currency"`
}

type SupplierProductInventory struct {
	Status string `json:"status"`
	Track  *bool  `json:"track_quantity"`
}

type SupplierProductWeight struct {
	Grams *int64  `json:"grams"`
	Unit  *string `json:"unit"`
}

type SupplierProductVolume struct {
	Value *int64  `json:"value"`
	Unit  *string `json:"unit"`
}

type SupplierProductDraftResult struct {
	Product ProductView `json:"product"`
	Created bool        `json:"created"`
}

type SupplierProductDraftWriter interface {
	StageSupplierProductDraft(ctx context.Context, shopID string, input SupplierProductDraftInput) (*SupplierProductDraftResult, error)
}

type CatalogReferenceWriter interface {
	ListBrands(ctx context.Context, shopID string) ([]BrandView, error)
	CreateBrand(ctx context.Context, shopID string, input BrandInput) (*BrandView, error)
	UpdateBrand(ctx context.Context, shopID, brandID string, input BrandInput) (*BrandView, error)
	ListSuppliers(ctx context.Context, shopID string) ([]SupplierView, error)
	CreateSupplier(ctx context.Context, shopID string, input SupplierInput) (*SupplierView, error)
	UpdateSupplier(ctx context.Context, shopID, supplierID string, input SupplierInput) (*SupplierView, error)
}

type BrandInput struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type BrandView struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type SupplierInput struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

type SupplierView struct {
	ID     string `json:"id"`
	Code   string `json:"code"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

func NewCatalogID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", fmt.Errorf("generate catalog id: %w", err)
	}
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(id[:])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:], nil
}
