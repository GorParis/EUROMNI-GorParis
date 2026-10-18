//go:build integration

package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"testing"
	"time"

	"euromni/internal/dbdialect"
	"euromni/internal/migrations"
	"euromni/internal/seed"
	schemafiles "euromni/migrations"
)

func TestMariaDBMigrationAndCommerceFlow(t *testing.T) {
	dsn := os.Getenv("MARIADB_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("MARIADB_TEST_DATABASE_URL must point to a dedicated MariaDB test database")
	}
	driverName, driverDSN, dialect, err := dbdialect.Resolve(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if dialect != dbdialect.MariaDB {
		t.Fatalf("MARIADB_TEST_DATABASE_URL resolved to %q, want MariaDB", dialect)
	}
	database, err := sql.Open(driverName, driverDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close MariaDB test database: %v", err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := database.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	source, err := fs.Sub(schemafiles.MariaDBFiles, "mariadb")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := migrations.ApplyMariaDB(ctx, database, source); err != nil {
		t.Fatalf("apply MariaDB migrations: %v", err)
	}

	store := NewSQLWithDialect(database, dbdialect.MariaDB)
	products, err := store.ListProducts(ctx, seed.ShopID)
	if err != nil {
		t.Fatalf("read seeded products: %v", err)
	}
	if len(products) < 5 {
		t.Fatalf("MariaDB seed created %d storefront products, want at least 5", len(products))
	}
	foundCabas := false
	for _, product := range products {
		if product.Slug == "cabas-lin" {
			foundCabas = true
			break
		}
	}
	if !foundCabas {
		t.Fatal("MariaDB seed is missing the cabas-lin storefront product")
	}
	categories, err := store.ListCategories(ctx, seed.ShopID)
	if err != nil {
		t.Fatalf("read seeded categories: %v", err)
	}
	foundSacs := len(categories) == 1 && categories[0].Slug == "sacs"
	if foundSacs {
		foundSacs = false
		for _, translation := range categories[0].Translations {
			if translation.Locale == "fr-FR" && translation.Name == "Sacs" {
				foundSacs = true
				break
			}
		}
	}
	if !foundSacs {
		t.Fatalf("unexpected MariaDB demo categories: %#v", categories)
	}
	module, err := store.GetInstalledModule(ctx, seed.ShopID, seed.ModuleID)
	if err != nil {
		t.Fatalf("read seeded module: %v", err)
	}
	if module.Name != "demo.loyalty" || len(module.Capabilities) != 4 {
		t.Fatalf("unexpected MariaDB demo module: %#v", module)
	}
	options, err := store.ListShippingOptions(ctx, seed.ShopID, "FR")
	if err != nil {
		t.Fatalf("read seeded shipping options: %v", err)
	}
	if len(options) == 0 || options[0].CarrierCode != "colissimo" ||
		options[0].ZoneCode != "france-ue" || options[0].ZoneName != "France et Union européenne" {
		t.Fatalf("unexpected seeded shipping options: %#v", options)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	product, err := store.CreateProduct(ctx, seed.ShopID, ProductInput{
		Slug:   "mariadb-test-" + suffix,
		Status: "active",
		Translations: []TranslationView{{
			Locale: "fr-FR", Name: "Produit MariaDB", Description: "Test d'intégration",
		}},
		Variants: []VariantInput{{
			SKU:       "MDB-" + suffix,
			Options:   map[string]any{"taille": "unique"},
			Prices:    []PriceView{{Currency: "EUR", Amount: "12.5000"}},
			Inventory: InventoryView{Quantity: 5, Track: true},
		}},
	})
	if err != nil {
		t.Fatalf("create MariaDB product: %v", err)
	}
	tokenHash := sha256.Sum256([]byte("mariadb-test-cart-" + suffix))
	email := "mariadb-test-" + suffix + "@example.invalid"
	var orderID string
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if orderID != "" {
			if _, err := database.ExecContext(cleanupCtx, `DELETE FROM orders WHERE id = $1`, orderID); err != nil {
				t.Errorf("clean up MariaDB test order: %v", err)
			}
		}
		if _, err := database.ExecContext(cleanupCtx, `DELETE FROM carts WHERE access_token_hash = $1`, tokenHash[:]); err != nil {
			t.Errorf("clean up MariaDB test cart: %v", err)
		}
		if _, err := database.ExecContext(cleanupCtx, `DELETE FROM customers WHERE shop_id = $1 AND email = $2`, seed.ShopID, email); err != nil {
			t.Errorf("clean up MariaDB test customer: %v", err)
		}
		if _, err := database.ExecContext(cleanupCtx, `DELETE FROM products WHERE id = $1`, product.ID); err != nil {
			t.Errorf("clean up MariaDB test product: %v", err)
		}
	})
	if err := store.CreateCart(ctx, seed.ShopID, tokenHash[:]); err != nil {
		t.Fatalf("create MariaDB cart: %v", err)
	}
	if _, err := store.AddCartLine(ctx, seed.ShopID, tokenHash[:], product.Variants[0].ID, 1); err != nil {
		t.Fatalf("add MariaDB cart line: %v", err)
	}
	order, err := store.CheckoutCart(ctx, seed.ShopID, tokenHash[:], CheckoutDetails{
		Email:               email,
		Name:                "MariaDB Integration",
		ShippingCarrierCode: "colissimo",
		ShippingAddress: ShippingAddress{
			Line1: "1 rue de test", City: "Paris", PostalCode: "75001", Country: "FR",
		},
	})
	if err != nil {
		t.Fatalf("checkout MariaDB cart: %v", err)
	}
	orderID = order.ID
	adminOrder, err := store.GetAdminOrder(ctx, seed.ShopID, order.ID)
	if err != nil {
		t.Fatalf("read MariaDB admin order: %v", err)
	}
	if adminOrder.Email != email || adminOrder.ShippingCarrierCode != "colissimo" {
		t.Fatalf("unexpected MariaDB order snapshot: %#v", adminOrder)
	}
	if err := store.CancelAdminOrder(ctx, seed.ShopID, order.ID); err != nil {
		t.Fatalf("cancel MariaDB test order and restore inventory: %v", err)
	}
}
