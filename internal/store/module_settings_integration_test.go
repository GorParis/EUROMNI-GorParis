//go:build integration

package store

import (
	"context"
	"errors"
	"testing"

	"euromni/internal/seed"
	"euromni/internal/testdb"
)

func TestPostgresNewProductsSettingsAreShopScoped(t *testing.T) {
	ctx := context.Background()
	database := testdb.Start(t)
	db := NewSQL(database)

	var shopID string
	if err := database.QueryRowContext(ctx, `
		INSERT INTO shops (slug, name) VALUES ('module-settings-test', 'Module Settings Test')
		RETURNING id::text
	`).Scan(&shopID); err != nil {
		t.Fatalf("create second test shop: %v", err)
	}
	var moduleID string
	if err := database.QueryRowContext(ctx, `
		INSERT INTO modules (name) VALUES ($1)
		RETURNING id::text
	`, NewProductsModuleName).Scan(&moduleID); err != nil {
		t.Fatalf("create new-products module: %v", err)
	}
	for _, shop := range []struct {
		id, config string
	}{
		{id: seed.ShopID, config: `{"days":30,"limit":8,"preserved":"first"}`},
		{id: shopID, config: `{"days":90,"limit":12,"preserved":"second"}`},
	} {
		if _, err := database.ExecContext(ctx, `
			INSERT INTO shop_modules (shop_id, module_id, status, config)
			VALUES ($1, $2, 'installed', $3::jsonb)
		`, shop.id, moduleID, shop.config); err != nil {
			t.Fatalf("install module for shop %s: %v", shop.id, err)
		}
	}

	first, err := db.GetNewProductsSettings(ctx, seed.ShopID)
	if err != nil || first != (NewProductsSettings{Days: 30, Limit: 8}) {
		t.Fatalf("first shop settings = %+v, error = %v", first, err)
	}
	second, err := db.GetNewProductsSettings(ctx, shopID)
	if err != nil || second != (NewProductsSettings{Days: 90, Limit: 12}) {
		t.Fatalf("second shop settings = %+v, error = %v", second, err)
	}

	updatedSettings := NewProductsSettings{Days: 0, Limit: 100}
	if err := db.UpdateNewProductsSettings(ctx, seed.ShopID, updatedSettings); err != nil {
		t.Fatalf("update first shop settings: %v", err)
	}
	first, err = db.GetNewProductsSettings(ctx, seed.ShopID)
	if err != nil || first != updatedSettings {
		t.Fatalf("updated first shop settings = %+v, error = %v", first, err)
	}
	second, err = db.GetNewProductsSettings(ctx, shopID)
	if err != nil || second != (NewProductsSettings{Days: 90, Limit: 12}) {
		t.Fatalf("second shop settings changed = %+v, error = %v", second, err)
	}
	var preserved string
	if err := database.QueryRowContext(ctx, `
		SELECT config->>'preserved'
		FROM shop_modules
		WHERE shop_id::text = $1 AND module_id::text = $2
	`, seed.ShopID, moduleID).Scan(&preserved); err != nil || preserved != "first" {
		t.Fatalf("unrelated config key = %q, error = %v", preserved, err)
	}

	if _, err := database.ExecContext(ctx, `
		UPDATE shop_modules SET status = 'disabled'
		WHERE shop_id::text = $1 AND module_id::text = $2
	`, shopID, moduleID); err != nil {
		t.Fatalf("disable module in second shop: %v", err)
	}
	if _, err := db.GetNewProductsSettings(ctx, shopID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("disabled module read error = %v, want not found", err)
	}
	if err := db.UpdateNewProductsSettings(ctx, shopID, updatedSettings); !errors.Is(err, ErrNotFound) {
		t.Fatalf("disabled module update error = %v, want not found", err)
	}
}
