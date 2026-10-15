//go:build integration

package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"testing"

	"euromni/internal/seed"
	"euromni/internal/testdb"
)

func TestPostgresStoreIntegration(t *testing.T) {
	ctx := context.Background()
	db := NewSQL(testdb.Start(t))

	product, err := db.GetProduct(ctx, seed.ShopID, seed.ProductID)
	if err != nil {
		t.Fatalf("read seeded product: %v", err)
	}
	if product.Slug != "cabas-lin" || len(product.Variants) != 1 {
		t.Fatalf("unexpected seeded product: %+v", product)
	}

	products, err := db.ListProducts(ctx, seed.ShopID)
	if err != nil {
		t.Fatalf("list seeded products: %v", err)
	}
	if len(products) != 5 || products[0].Slug != "bougie-cedre" || products[2].Slug != "ceinture-cuir" || len(products[2].Variants) != 3 {
		t.Fatalf("unexpected catalog list: %+v", products)
	}
	if len(products[0].Variants) != 1 || len(products[0].Variants[0].Options) != 0 || len(products[1].Variants) != 1 || len(products[3].Variants) != 3 || len(products[4].Variants) != 1 || len(products[4].Variants[0].Options) != 0 {
		t.Fatalf("unexpected mix of single-SKU and variant products: %+v", products)
	}

	categories, err := db.ListCategories(ctx, seed.ShopID)
	if err != nil {
		t.Fatalf("read seeded categories: %v", err)
	}
	if len(categories) != 1 || categories[0].ID != seed.CategoryID || categories[0].Slug != "sacs" {
		t.Fatalf("unexpected seeded categories: %+v", categories)
	}

	category, err := db.GetCategory(ctx, seed.ShopID, seed.CategoryID)
	if err != nil {
		t.Fatalf("read seeded category: %v", err)
	}
	if len(category.Translations) != 2 {
		t.Fatalf("category translations = %d, want 2", len(category.Translations))
	}
	if _, err := db.AuthenticateStaff(ctx, seed.ShopID, "owner@atelier-demo.example", "integration-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("authenticate unconfigured seeded owner error = %v, want %v", err, ErrInvalidCredentials)
	}
	if err := db.BootstrapStaff(ctx, seed.ShopID, "wrong-owner@atelier-demo.example", "Wrong Owner", "integration-password"); !errors.Is(err, ErrStaffAlreadyBootstrapped) {
		t.Fatalf("bootstrap seeded owner with wrong email error = %v, want %v", err, ErrStaffAlreadyBootstrapped)
	}
	if err := db.BootstrapStaff(ctx, seed.ShopID, "owner@atelier-demo.example", "Demo Owner", "integration-password"); err != nil {
		t.Fatalf("bootstrap seeded owner: %v", err)
	}
	if err := db.BootstrapStaff(ctx, seed.ShopID, "owner@atelier-demo.example", "Demo Owner", "another-password"); !errors.Is(err, ErrStaffAlreadyBootstrapped) {
		t.Fatalf("second staff bootstrap error = %v, want %v", err, ErrStaffAlreadyBootstrapped)
	}
	staffSession, err := db.AuthenticateStaff(ctx, seed.ShopID, "owner@atelier-demo.example", "integration-password")
	if err != nil {
		t.Fatalf("authenticate bootstrapped owner: %v", err)
	}
	if _, err := db.ValidateStaffSession(ctx, staffSession.Token); err != nil {
		t.Fatalf("validate staff session: %v", err)
	}
	if err := db.RevokeStaffSession(ctx, staffSession.Token); err != nil {
		t.Fatalf("revoke staff session: %v", err)
	}
	if _, err := db.ValidateStaffSession(ctx, staffSession.Token); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("revoked staff session error = %v, want %v", err, ErrInvalidCredentials)
	}
	expiredSession, err := db.AuthenticateStaff(ctx, seed.ShopID, "owner@atelier-demo.example", "integration-password")
	if err != nil {
		t.Fatalf("authenticate owner for expiry test: %v", err)
	}
	expiredHash := sha256.Sum256([]byte(expiredSession.Token))
	if _, err := db.SQL.ExecContext(ctx, `UPDATE sessions SET expires_at = now() - interval '1 second' WHERE token_hash = $1`, expiredHash[:]); err != nil {
		t.Fatalf("expire staff session: %v", err)
	}
	if _, err := db.ValidateStaffSession(ctx, expiredSession.Token); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expired staff session error = %v, want %v", err, ErrInvalidCredentials)
	}
	if err := db.RevokeStaffSession(ctx, expiredSession.Token); err != nil {
		t.Fatalf("revoke expired staff session: %v", err)
	}

	emptyShopID := "e0e00000-0001-4000-8000-000000000099"
	if _, err := db.SQL.ExecContext(ctx, `INSERT INTO shops (id, slug, name) VALUES ($1, 'bootstrap-test', 'Bootstrap test')`, emptyShopID); err != nil {
		t.Fatalf("create empty bootstrap shop: %v", err)
	}
	if err := db.BootstrapStaff(ctx, emptyShopID, "new-owner@example.com", "New Owner", "empty-shop-password"); err != nil {
		t.Fatalf("bootstrap owner in empty shop: %v", err)
	}
	if err := db.BootstrapStaff(ctx, emptyShopID, "another-owner@example.com", "Another Owner", "another-password"); !errors.Is(err, ErrStaffAlreadyBootstrapped) {
		t.Fatalf("second empty-shop bootstrap error = %v, want %v", err, ErrStaffAlreadyBootstrapped)
	}
	emptyShopSession, err := db.AuthenticateStaff(ctx, emptyShopID, "new-owner@example.com", "empty-shop-password")
	if err != nil || emptyShopSession.Role != "owner" {
		t.Fatalf("authenticate empty-shop owner = %+v, error = %v", emptyShopSession, err)
	}

	adminProduct, err := db.CreateProduct(ctx, seed.ShopID, ProductInput{
		Slug: "admin-crud-test", Status: "draft", CategoryIDs: []string{seed.CategoryID},
		Translations: []TranslationView{{Locale: "fr-FR", Name: "Article admin", Description: "Brouillon"}},
		Variants: []VariantInput{{
			SKU: "ADMIN-CRUD-TEST", Options: map[string]any{"taille": "unique"},
			Prices:    []PriceView{{Currency: "EUR", Amount: "12.5000"}},
			Inventory: InventoryView{Quantity: 8, Track: true},
		}},
	})
	if err != nil {
		t.Fatalf("create admin product: %v", err)
	}
	if len(adminProduct.CategoryIDs) != 1 || adminProduct.CategoryIDs[0] != seed.CategoryID || len(adminProduct.Variants) != 1 ||
		adminProduct.Variants[0].SKU != "ADMIN-CRUD-TEST" || adminProduct.Variants[0].Prices[0].Amount != "12.5000" ||
		adminProduct.Variants[0].Inventory.Quantity != 8 {
		t.Fatalf("created admin product associations and inventory = %+v", adminProduct)
	}
	if _, err := db.CreateProduct(ctx, seed.ShopID, ProductInput{
		Slug: "cabas-lin", Status: "draft",
		Translations: []TranslationView{{Locale: "fr-FR", Name: "Doublon"}},
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate product error = %v, want %v", err, ErrConflict)
	}
	adminProducts, err := db.ListAdminProducts(ctx, seed.ShopID)
	if err != nil || len(adminProducts) != 6 {
		t.Fatalf("admin products = %d, error = %v; want 6 including draft", len(adminProducts), err)
	}
	updatedProduct, err := db.UpdateProduct(ctx, seed.ShopID, adminProduct.ID, ProductInput{
		Slug: "admin-crud-test", Status: "active", CategoryIDs: []string{seed.CategoryID},
		Translations: []TranslationView{{Locale: "fr-FR", Name: "Article publié", Description: "Mis à jour"}},
		Variants: []VariantInput{{
			SKU: "ADMIN-CRUD-TEST", Options: map[string]any{"taille": "unique"},
			Prices:    []PriceView{{Currency: "EUR", Amount: "15.0000"}},
			Inventory: InventoryView{Quantity: 6, Track: true},
		}},
	})
	if err != nil || updatedProduct.Status != "active" || updatedProduct.Translations[0].Name != "Article publié" {
		t.Fatalf("updated product = %+v, error = %v", updatedProduct, err)
	}
	if len(updatedProduct.CategoryIDs) != 1 || len(updatedProduct.Variants) != 1 ||
		updatedProduct.Variants[0].ID != adminProduct.Variants[0].ID ||
		updatedProduct.Variants[0].Prices[0].Amount != "15.0000" || updatedProduct.Variants[0].Inventory.Quantity != 6 {
		t.Fatalf("updated admin product associations and inventory = %+v", updatedProduct)
	}
	adminCartToken := sha256.Sum256([]byte("admin variant reference cart"))
	if err := db.CreateCart(ctx, seed.ShopID, adminCartToken[:]); err != nil {
		t.Fatalf("create admin variant reference cart: %v", err)
	}
	defer func() {
		_, _ = db.SQL.ExecContext(context.Background(), "DELETE FROM carts WHERE access_token_hash = $1", adminCartToken[:])
	}()
	if _, err := db.AddCartLine(ctx, seed.ShopID, adminCartToken[:], updatedProduct.Variants[0].ID, 1); err != nil {
		t.Fatalf("add admin variant to cart: %v", err)
	}
	if _, err := db.UpdateProduct(ctx, seed.ShopID, adminProduct.ID, ProductInput{
		Slug: "admin-crud-test", Status: "active",
		Translations: []TranslationView{{Locale: "fr-FR", Name: "Article publié"}},
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("removing cart-referenced variant error = %v, want %v", err, ErrConflict)
	}
	if _, err := db.CheckoutCart(ctx, seed.ShopID, adminCartToken[:], CheckoutDetails{
		Email: "admin-variant-order@example.com",
		Name:  "Admin variant order",
		ShippingAddress: ShippingAddress{
			Line1: "1 rue du Test", City: "Paris", PostalCode: "75001", Country: "FR",
		},
	}); err != nil {
		t.Fatalf("checkout cart containing admin variant: %v", err)
	}
	if _, err := db.UpdateProduct(ctx, seed.ShopID, adminProduct.ID, ProductInput{
		Slug: "admin-crud-test", Status: "active",
		Translations: []TranslationView{{Locale: "fr-FR", Name: "Article publié"}},
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("removing order-referenced variant error = %v, want %v", err, ErrConflict)
	}
	if err := db.ArchiveProduct(ctx, seed.ShopID, adminProduct.ID); err != nil {
		t.Fatalf("archive admin product: %v", err)
	}
	if _, err := db.GetProduct(ctx, seed.ShopID, adminProduct.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("read archived product error = %v, want %v", err, ErrNotFound)
	}

	adminCategory, err := db.CreateCategory(ctx, seed.ShopID, CategoryInput{
		Slug:         "admin-crud-test",
		Translations: []CategoryTranslationView{{Locale: "fr-FR", Name: "Catégorie admin"}},
	})
	if err != nil {
		t.Fatalf("create admin category: %v", err)
	}
	if _, err := db.CreateCategory(ctx, seed.ShopID, CategoryInput{
		Slug: "sacs", Translations: []CategoryTranslationView{{Locale: "fr-FR", Name: "Doublon"}},
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate category error = %v, want %v", err, ErrConflict)
	}
	updatedCategory, err := db.UpdateCategory(ctx, seed.ShopID, adminCategory.ID, CategoryInput{
		Slug:         "admin-crud-modified",
		Translations: []CategoryTranslationView{{Locale: "fr-FR", Name: "Catégorie modifiée"}},
	})
	if err != nil || updatedCategory.Slug != "admin-crud-modified" {
		t.Fatalf("updated category = %+v, error = %v", updatedCategory, err)
	}
	if err := db.DeleteCategory(ctx, seed.ShopID, adminCategory.ID); err != nil {
		t.Fatalf("delete admin category: %v", err)
	}
	if _, err := db.GetCategory(ctx, seed.ShopID, adminCategory.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("read deleted category error = %v, want %v", err, ErrNotFound)
	}
	var migrationCount int
	if err := db.SQL.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations").Scan(&migrationCount); err != nil {
		t.Fatalf("read migration ledger: %v", err)
	}
	if migrationCount != 6 {
		t.Fatalf("recorded migrations = %d, want 6", migrationCount)
	}
	installedModule, err := db.GetInstalledModule(ctx, seed.ShopID, seed.ModuleID)
	if err != nil {
		t.Fatalf("read installed module: %v", err)
	}
	if installedModule.Name != "demo.loyalty" || installedModule.ABIVersion != "euromni.abi.v1" || installedModule.WASMSHA256 != nil {
		t.Fatalf("installed module = %+v, want demo module without registered artifact hash", installedModule)
	}
	expectedCapabilities := map[string]bool{"catalog": false, "customer": false, "hooks": false, "module.kv": false}
	for _, capability := range installedModule.Capabilities {
		if _, exists := expectedCapabilities[capability]; exists {
			expectedCapabilities[capability] = true
		}
	}
	for capability, found := range expectedCapabilities {
		if !found {
			t.Errorf("installed module missing capability %q: %v", capability, installedModule.Capabilities)
		}
	}

	rawToken := make([]byte, 32)
	if _, err := rand.Read(rawToken); err != nil {
		t.Fatal("generate cart token failed")
	}
	tokenHash := sha256.Sum256(rawToken)
	if err := db.CreateCart(ctx, seed.ShopID, tokenHash[:]); err != nil {
		t.Fatalf("create cart: %v", err)
	}
	defer func() {
		_, _ = db.SQL.ExecContext(context.Background(), "DELETE FROM carts WHERE access_token_hash = $1", tokenHash[:])
	}()

	cart, err := db.AddCartLine(ctx, seed.ShopID, tokenHash[:], seed.VariantID, 2)
	if err != nil {
		t.Fatalf("add seeded variant: %v", err)
	}
	if len(cart.Items) != 1 || cart.Items[0].Quantity != 2 {
		t.Fatalf("cart items = %+v, want one line with quantity 2", cart.Items)
	}
	if cart.Total.Amount != "178.0000" || cart.Total.Currency != "EUR" {
		t.Fatalf("cart total = %+v, want EUR 178.0000", cart.Total)
	}
	if _, err := db.AddCartLine(ctx, seed.ShopID, tokenHash[:], seed.VariantID, 23); err != ErrOutOfStock {
		t.Fatalf("overstock error = %v, want %v", err, ErrOutOfStock)
	}

	order, err := db.CheckoutCart(ctx, seed.ShopID, tokenHash[:], CheckoutDetails{
		Email: "checkout@example.com",
		Name:  "Checkout test",
		ShippingAddress: ShippingAddress{
			Line1: "1 rue du Test", City: "Paris", PostalCode: "75001", Country: "FR",
		},
	})
	if err != nil {
		t.Fatalf("checkout cart: %v", err)
	}
	if order.Status != "pending" || order.Subtotal != "178.0000" || len(order.Items) != 1 || order.Items[0].SKU != "CABAS-LIN-NAT" {
		t.Fatalf("unexpected order result: %+v", order)
	}
	var remainingStock, savedLineCount, savedLineQuantity, savedOrders int
	if err := db.SQL.QueryRowContext(ctx, `SELECT quantity FROM inventory_items WHERE variant_id::text = $1`, seed.VariantID).Scan(&remainingStock); err != nil {
		t.Fatalf("read remaining stock: %v", err)
	}
	if err := db.SQL.QueryRowContext(ctx, `SELECT count(*), COALESCE(sum(quantity), 0)::int FROM order_lines WHERE order_id::text = $1 AND name = 'Cabas en lin' AND unit_price = 89 AND line_total = 178`, order.ID).Scan(&savedLineCount, &savedLineQuantity); err != nil {
		t.Fatalf("read order line snapshot: %v", err)
	}
	if err := db.SQL.QueryRowContext(ctx, `SELECT count(*) FROM orders WHERE id::text = $1 AND shipping_address->>'country' = 'FR'`, order.ID).Scan(&savedOrders); err != nil {
		t.Fatalf("read saved order: %v", err)
	}
	if remainingStock != 22 || savedLineCount != 1 || savedLineQuantity != 2 || savedOrders != 1 {
		t.Fatalf("checkout persistence stock/orders/lines/quantity = %d/%d/%d/%d, want 22/1/1/2", remainingStock, savedOrders, savedLineCount, savedLineQuantity)
	}
	if _, err := db.CheckoutCart(ctx, seed.ShopID, tokenHash[:], CheckoutDetails{}); err != ErrEmptyCart {
		t.Fatalf("second checkout error = %v, want %v", err, ErrEmptyCart)
	}
}
