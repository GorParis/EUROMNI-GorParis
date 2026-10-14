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
	Slug         string            `json:"slug"`
	Status       string            `json:"status"`
	Translations []TranslationView `json:"translations"`
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
