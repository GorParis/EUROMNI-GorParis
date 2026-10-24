//go:build integration

package store

import (
	"context"
	"errors"
	"testing"

	"euromni/internal/seed"
	"euromni/internal/testdb"
)

func TestPostgresStageSupplierProductDraftIsIdempotentAndShopScoped(t *testing.T) {
	ctx := context.Background()
	database := testdb.Start(t)
	db := NewSQL(database)
	input := validSupplierProductDraftInput()

	first, err := db.StageSupplierProductDraft(ctx, seed.ShopID, input)
	if err != nil {
		t.Fatalf("stage first supplier product draft: %v", err)
	}
	if !first.Created || first.Product.Status != "draft" || first.Product.Slug != "selfnamed-842003" ||
		first.Product.Volume == nil || first.Product.Volume.Value != 88 || first.Product.Volume.Unit != nil ||
		len(first.Product.Tags) != 2 || first.Product.Variants[0].Inventory.StockStatus != "out_of_stock" ||
		first.Product.Variants[0].Inventory.Track {
		t.Fatalf("staged product does not retain preview fields safely: %+v", first.Product)
	}
	if len(first.Product.Translations) != 1 || first.Product.Translations[0].Locale != "fr-FR" ||
		first.Product.Variants[0].Prices[0].Currency != "EUR" {
		t.Fatalf("staged product did not use shop locale/currency: %+v", first.Product)
	}

	input.Name = "Updated synthetic balm"
	input.Price.Amount = "18"
	second, err := db.StageSupplierProductDraft(ctx, seed.ShopID, input)
	if err != nil {
		t.Fatalf("restage supplier product draft: %v", err)
	}
	if second.Created || second.Product.ID != first.Product.ID ||
		second.Product.Translations[0].Name != input.Name ||
		second.Product.Variants[0].Prices[0].Amount != "18.0000" {
		t.Fatalf("restaging did not idempotently update the draft: %+v", second)
	}

	otherShopID, err := NewCatalogID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO shops (id, slug, name) VALUES ($1, $2, 'Second supplier draft shop')
	`, otherShopID, "supplier-draft-"+otherShopID); err != nil {
		t.Fatalf("create second shop: %v", err)
	}
	otherShopDraft, err := db.StageSupplierProductDraft(ctx, otherShopID, input)
	if err != nil {
		t.Fatalf("stage same external product for second shop: %v", err)
	}
	if !otherShopDraft.Created || otherShopDraft.Product.ID == first.Product.ID {
		t.Fatalf("supplier reference crossed shop boundary: first=%s second=%+v", first.Product.ID, otherShopDraft)
	}

	if _, err := database.ExecContext(ctx, `
		UPDATE products SET status = 'active' WHERE shop_id = $1 AND id = $2
	`, seed.ShopID, first.Product.ID); err != nil {
		t.Fatalf("activate staged product for conflict check: %v", err)
	}
	if _, err := db.StageSupplierProductDraft(ctx, seed.ShopID, input); !errors.Is(err, ErrConflict) {
		t.Fatalf("restage active supplier product error = %v, want ErrConflict", err)
	}
}
